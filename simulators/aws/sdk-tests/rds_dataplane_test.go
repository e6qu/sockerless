package aws_sdk_test

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	rdsauth "github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRDSNativeDataPlanesWithIAMAuthentication_SDK creates database instances
// through the official Amazon RDS SDK, generates their IAM database
// authentication tokens through AWS's official signer, and then executes
// schema/write/read transactions through stock PostgreSQL and MySQL drivers.
// The driver connections are the independent data-plane oracle; the simulator
// test does not call an internal database endpoint.
func TestRDSNativeDataPlanesWithIAMAuthentication_SDK(t *testing.T) {
	testContext, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	rdsAPI := rdsClient()
	iamAPI := iamClient()
	const (
		iamUser   = "rds-database-client"
		iamPolicy = "rds-database-connect"
	)
	_, err := iamAPI.CreateUser(testContext, &iam.CreateUserInput{UserName: aws.String(iamUser)})
	require.NoError(t, err)
	accessKey, err := iamAPI.CreateAccessKey(testContext, &iam.CreateAccessKeyInput{UserName: aws.String(iamUser)})
	require.NoError(t, err)
	accessKeyID := aws.ToString(accessKey.AccessKey.AccessKeyId)
	secretAccessKey := aws.ToString(accessKey.AccessKey.SecretAccessKey)
	t.Cleanup(func() {
		_, _ = iamAPI.DeleteUserPolicy(context.Background(), &iam.DeleteUserPolicyInput{
			UserName: aws.String(iamUser), PolicyName: aws.String(iamPolicy),
		})
		_, _ = iamAPI.DeleteAccessKey(context.Background(), &iam.DeleteAccessKeyInput{
			UserName: aws.String(iamUser), AccessKeyId: aws.String(accessKeyID),
		})
		_, _ = iamAPI.DeleteUser(context.Background(), &iam.DeleteUserInput{UserName: aws.String(iamUser)})
	})
	credentialProvider := credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")

	t.Run("PostgreSQL", func(t *testing.T) {
		const (
			instanceID = "rds-postgresql-wire"
			username   = "dbadmin"
			database   = "application"
		)
		created, err := rdsAPI.CreateDBInstance(testContext, &rds.CreateDBInstanceInput{
			DBInstanceIdentifier:            aws.String(instanceID),
			DBInstanceClass:                 aws.String("db.t3.micro"),
			Engine:                          aws.String("postgres"),
			AllocatedStorage:                aws.Int32(20),
			MasterUsername:                  aws.String(username),
			MasterUserPassword:              aws.String("MasterPassword-123!"),
			DBName:                          aws.String(database),
			EnableIAMDatabaseAuthentication: aws.Bool(true),
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = rdsAPI.DeleteDBInstance(context.Background(), &rds.DeleteDBInstanceInput{
				DBInstanceIdentifier: aws.String(instanceID), SkipFinalSnapshot: aws.Bool(true),
			})
		})
		endpoint := fmt.Sprintf("%s:%d", aws.ToString(created.DBInstance.Endpoint.Address), aws.ToInt32(created.DBInstance.Endpoint.Port))
		token, err := rdsauth.BuildAuthToken(testContext, endpoint, "us-east-1", username, credentialProvider)
		require.NoError(t, err)

		config, err := pgx.ParseConfig(fmt.Sprintf(
			"postgres://%s@%s/%s?sslmode=require", username, endpoint, database,
		))
		require.NoError(t, err)
		config.Password = token
		config.TLSConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12} // test-only CA coordinate
		deniedConnection, err := pgx.ConnectConfig(testContext, config)
		require.Error(t, err, "a valid IAM database token without rds-db:connect must be denied")
		if deniedConnection != nil {
			_ = deniedConnection.Close(context.Background())
		}
		_, err = iamAPI.PutUserPolicy(testContext, &iam.PutUserPolicyInput{
			UserName:   aws.String(iamUser),
			PolicyName: aws.String(iamPolicy),
			PolicyDocument: aws.String(
				`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"rds-db:connect","Resource":"*"}]}`,
			),
		})
		require.NoError(t, err)
		connection, err := pgx.ConnectConfig(testContext, config)
		require.NoError(t, err)
		defer connection.Close(context.Background())

		_, err = connection.Exec(testContext, `CREATE TABLE fidelity (id integer PRIMARY KEY, value text NOT NULL)`)
		require.NoError(t, err)
		_, err = connection.Exec(testContext, `INSERT INTO fidelity (id, value) VALUES (1, 'postgres-real-engine')`)
		require.NoError(t, err)
		var value string
		require.NoError(t, connection.QueryRow(testContext, `SELECT value FROM fidelity WHERE id = 1`).Scan(&value))
		assert.Equal(t, "postgres-real-engine", value)
	})

	t.Run("MySQL", func(t *testing.T) {
		const (
			instanceID = "rds-mysql-wire"
			username   = "dbadmin"
			database   = "application"
		)
		created, err := rdsAPI.CreateDBInstance(testContext, &rds.CreateDBInstanceInput{
			DBInstanceIdentifier:            aws.String(instanceID),
			DBInstanceClass:                 aws.String("db.t3.micro"),
			Engine:                          aws.String("mysql"),
			AllocatedStorage:                aws.Int32(20),
			MasterUsername:                  aws.String(username),
			MasterUserPassword:              aws.String("MasterPassword-123!"),
			DBName:                          aws.String(database),
			EnableIAMDatabaseAuthentication: aws.Bool(true),
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = rdsAPI.DeleteDBInstance(context.Background(), &rds.DeleteDBInstanceInput{
				DBInstanceIdentifier: aws.String(instanceID), SkipFinalSnapshot: aws.Bool(true),
			})
		})
		endpoint := fmt.Sprintf("%s:%d", aws.ToString(created.DBInstance.Endpoint.Address), aws.ToInt32(created.DBInstance.Endpoint.Port))
		token, err := rdsauth.BuildAuthToken(testContext, endpoint, "us-east-1", username, credentialProvider)
		require.NoError(t, err)
		config := mysql.Config{
			User: username, Passwd: token, Net: "tcp", Addr: endpoint, DBName: database,
			TLSConfig: "skip-verify", AllowCleartextPasswords: true,
		}
		databaseConnection, err := sql.Open("mysql", config.FormatDSN())
		require.NoError(t, err)
		defer databaseConnection.Close()
		require.NoError(t, databaseConnection.PingContext(testContext))

		_, err = databaseConnection.ExecContext(testContext, `CREATE TABLE fidelity (id integer PRIMARY KEY, value varchar(255) NOT NULL)`)
		require.NoError(t, err)
		_, err = databaseConnection.ExecContext(testContext, `INSERT INTO fidelity (id, value) VALUES (1, 'mysql-real-engine')`)
		require.NoError(t, err)
		var value string
		require.NoError(t, databaseConnection.QueryRowContext(testContext, `SELECT value FROM fidelity WHERE id = 1`).Scan(&value))
		assert.Equal(t, "mysql-real-engine", value)
	})
}
