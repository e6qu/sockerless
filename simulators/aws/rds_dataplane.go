package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

const rdsAWSOwnedKMSKeyID = "aws-owned-rds"
const rdsEmptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type rdsDataPlaneRuntime struct {
	instance RDSInstance
	listener net.Listener
	backend  string
	handle   *sim.ContainerHandle
	start    sync.Once
	startErr error
}

var (
	rdsDataPlanes sync.Map
	rdsTLSOnce    sync.Once
	rdsTLSCert    tls.Certificate
	rdsTLSErr     error
)

func rdsInstallDataPlane(instance *RDSInstance, masterPassword string) error {
	engine := strings.ToLower(instance.Engine)
	if !strings.HasPrefix(engine, "postgres") && engine != "mysql" && engine != "mariadb" {
		return nil
	}
	if masterPassword == "" {
		return fmt.Errorf("MasterUserPassword is required for the %s data plane", instance.Engine)
	}
	if _, ok := kmsGetKeyMaterial(rdsAWSOwnedKMSKeyID); !ok {
		if _, err := kmsGenerateKeyMaterial(rdsAWSOwnedKMSKeyID); err != nil {
			return fmt.Errorf("generate AWS owned RDS key: %w", err)
		}
	}
	ciphertext, ok := kmsEncryptBytes(rdsAWSOwnedKMSKeyID, []byte(masterPassword))
	if !ok {
		return fmt.Errorf("encrypt RDS master-user credential")
	}
	instance.MasterUserSecret = ciphertext

	listener, err := rdsListenOnLoopback(instance.DBInstanceIdentifier, instance.Port)
	if err != nil {
		return fmt.Errorf("allocate RDS endpoint: %w", err)
	}
	listenAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return fmt.Errorf("RDS endpoint listener returned address type %T", listener.Addr())
	}
	instance.Endpoint = listenAddress.IP.String()
	instance.Port = listenAddress.Port
	runtime := &rdsDataPlaneRuntime{instance: *instance, listener: listener}
	rdsDataPlanes.Store(instance.DBInstanceIdentifier, runtime)
	go runtime.serve()
	return nil
}

func rdsListenOnLoopback(identifier string, port int) (net.Listener, error) {
	var seed byte = 2
	for i := 0; i < len(identifier); i++ {
		seed += identifier[i]
	}
	for offset := 0; offset < 253; offset++ {
		octet := 2 + (int(seed)+offset)%253
		address := net.JoinHostPort(fmt.Sprintf("127.0.0.%d", octet), strconv.Itoa(port))
		listener, err := net.Listen("tcp", address)
		if err == nil {
			return listener, nil
		}
	}
	// A host database or another simulator may already bind the engine's
	// conventional port on all loopback addresses. The endpoint port is a
	// coordinate, so ask the kernel for an isolated one rather than sharing or
	// replacing that listener.
	return net.Listen("tcp", "127.0.0.1:0")
}

func (runtime *rdsDataPlaneRuntime) serve() {
	for {
		connection, err := runtime.listener.Accept()
		if err != nil {
			return
		}
		go runtime.serveConnection(connection)
	}
}

func (runtime *rdsDataPlaneRuntime) ensureBackend() error {
	runtime.start.Do(func() {
		runtime.startErr = runtime.startBackend()
	})
	return runtime.startErr
}

func (runtime *rdsDataPlaneRuntime) startBackend() error {
	_, password, ok := kmsDecryptBytes(runtime.instance.MasterUserSecret)
	if !ok {
		return fmt.Errorf("RDS master-user credential could not be decrypted")
	}
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("allocate database engine port: %w", err)
	}
	backendAddress, ok := backendListener.Addr().(*net.TCPAddr)
	if !ok {
		_ = backendListener.Close()
		return fmt.Errorf("database engine listener returned address type %T", backendListener.Addr())
	}
	backendPort := backendAddress.Port
	if err := backendListener.Close(); err != nil {
		return fmt.Errorf("release database engine port: %w", err)
	}
	database := runtime.instance.DBName
	if database == "" {
		database = runtime.instance.MasterUsername
	}
	engine := strings.ToLower(runtime.instance.Engine)
	var image string
	var containerPort int
	var env map[string]string
	var args []string
	var dataPath string
	switch {
	case strings.HasPrefix(engine, "postgres"):
		image = "public.ecr.aws/docker/library/postgres:16-alpine"
		containerPort = 5432
		dataPath = "/var/lib/postgresql/data"
		env = map[string]string{
			"POSTGRES_USER":             runtime.instance.MasterUsername,
			"POSTGRES_PASSWORD":         string(password),
			"POSTGRES_DB":               database,
			"POSTGRES_HOST_AUTH_METHOD": "trust",
		}
	case engine == "mysql", engine == "mariadb":
		image = "public.ecr.aws/docker/library/mysql:8.0"
		containerPort = 3306
		dataPath = "/var/lib/mysql"
		env = map[string]string{
			"MYSQL_USER":          runtime.instance.MasterUsername,
			"MYSQL_PASSWORD":      string(password),
			"MYSQL_ROOT_PASSWORD": string(password),
			"MYSQL_DATABASE":      database,
		}
		args = []string{"--default-authentication-plugin=mysql_native_password"}
	default:
		return fmt.Errorf("database engine %q has no data plane", runtime.instance.Engine)
	}
	handle, err := sim.StartContainerSync(sim.ContainerConfig{
		Image: image, Architecture: "linux/amd64", Args: args, Env: env,
		PublishPorts: map[int]int{containerPort: backendPort},
		Binds:        []string{"sockerless-rds-" + runtime.instance.DBInstanceIdentifier + ":" + dataPath},
		Labels:       map[string]string{"sockerless-rds-instance": runtime.instance.DBInstanceIdentifier},
		Sandbox:      sim.SandboxFargate,
	}, sim.NoopSink{})
	if err != nil {
		return fmt.Errorf("start %s database engine: %w", runtime.instance.Engine, err)
	}
	runtime.handle = handle
	runtime.backend = net.JoinHostPort("127.0.0.1", strconv.Itoa(backendPort))
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if rdsDatabaseEngineReady(engine, runtime.backend, runtime.instance.MasterUsername, database) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	handle.Cancel()
	return fmt.Errorf("%s database engine did not become ready", runtime.instance.Engine)
}

func rdsDatabaseEngineReady(engine, address, username, database string) bool {
	connection, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if strings.HasPrefix(engine, "postgres") {
		parameters := []byte("user\x00" + username + "\x00database\x00" + database + "\x00\x00")
		packet := make([]byte, 8+len(parameters))
		binary.BigEndian.PutUint32(packet[:4], uint32(len(packet)))
		binary.BigEndian.PutUint32(packet[4:8], 196608)
		copy(packet[8:], parameters)
		if _, err := connection.Write(packet); err != nil {
			return false
		}
		header := make([]byte, 5)
		_, err := io.ReadFull(connection, header)
		return err == nil && (header[0] == 'R' || header[0] == 'E')
	}
	_, payload, err := mysqlReadPacket(connection)
	return err == nil && len(payload) > 0 && payload[0] == 10
}

func (runtime *rdsDataPlaneRuntime) serveConnection(client net.Conn) {
	defer client.Close()
	if err := runtime.ensureBackend(); err != nil {
		log.Printf("Amazon RDS %s data plane: %v", runtime.instance.DBInstanceIdentifier, err)
		return
	}
	if strings.HasPrefix(strings.ToLower(runtime.instance.Engine), "postgres") {
		runtime.servePostgreSQL(client)
		return
	}
	runtime.serveMySQL(client)
}

func (runtime *rdsDataPlaneRuntime) servePostgreSQL(client net.Conn) {
	startup, secure, err := rdsReadPostgreSQLStartup(client)
	if err != nil {
		log.Printf("Amazon RDS %s PostgreSQL startup: %v", runtime.instance.DBInstanceIdentifier, err)
		return
	}
	if secure != nil {
		client = secure
		defer client.Close()
	}
	user := rdsPostgreSQLStartupParameter(startup, "user")
	if err := rdsPostgreSQLAuthenticate(client, runtime.instance, user); err != nil {
		rdsPostgreSQLAuthError(client, err.Error())
		return
	}
	backend, err := net.DialTimeout("tcp", runtime.backend, 5*time.Second)
	if err != nil {
		log.Printf("Amazon RDS %s PostgreSQL backend dial: %v", runtime.instance.DBInstanceIdentifier, err)
		return
	}
	defer backend.Close()
	if _, err := backend.Write(startup); err != nil {
		log.Printf("Amazon RDS %s PostgreSQL backend startup: %v", runtime.instance.DBInstanceIdentifier, err)
		return
	}
	rdsRelay(client, backend)
}

func rdsReadPostgreSQLStartup(connection net.Conn) ([]byte, net.Conn, error) {
	packet, err := rdsReadPostgreSQLPacket(connection)
	if err != nil {
		return nil, nil, err
	}
	if len(packet) == 8 && binary.BigEndian.Uint32(packet[4:]) == 80877103 {
		if _, err := connection.Write([]byte{'S'}); err != nil {
			return nil, nil, err
		}
		certificate, err := rdsServerCertificate()
		if err != nil {
			return nil, nil, err
		}
		secure := tls.Server(connection, &tls.Config{
			Certificates: []tls.Certificate{certificate},
			MinVersion:   tls.VersionTLS12,
		})
		if err := secure.Handshake(); err != nil {
			return nil, nil, err
		}
		packet, err = rdsReadPostgreSQLPacket(secure)
		return packet, secure, err
	}
	return packet, nil, nil
}

func rdsReadPostgreSQLPacket(connection net.Conn) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(connection, header); err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint32(header))
	if length < 8 || length > 1<<20 {
		return nil, fmt.Errorf("invalid PostgreSQL startup packet length %d", length)
	}
	packet := make([]byte, length)
	copy(packet, header)
	_, err := io.ReadFull(connection, packet[4:])
	return packet, err
}

func rdsPostgreSQLStartupParameter(packet []byte, wanted string) string {
	if len(packet) < 9 {
		return ""
	}
	fields := strings.Split(string(packet[8:len(packet)-1]), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == wanted {
			return fields[i+1]
		}
	}
	return ""
}

func rdsPostgreSQLAuthenticate(connection net.Conn, instance RDSInstance, user string) error {
	request := make([]byte, 9)
	request[0] = 'R'
	binary.BigEndian.PutUint32(request[1:5], 8)
	binary.BigEndian.PutUint32(request[5:9], 3)
	if _, err := connection.Write(request); err != nil {
		return err
	}
	header := make([]byte, 5)
	if _, err := io.ReadFull(connection, header); err != nil {
		return err
	}
	if header[0] != 'p' {
		return fmt.Errorf("password authentication failed for user %q", user)
	}
	length := int(binary.BigEndian.Uint32(header[1:]))
	if length < 5 || length > 1<<20 {
		return fmt.Errorf("password authentication failed for user %q", user)
	}
	body := make([]byte, length-4)
	if _, err := io.ReadFull(connection, body); err != nil {
		return err
	}
	password := strings.TrimSuffix(string(body), "\x00")
	_, masterPassword, ok := kmsDecryptBytes(instance.MasterUserSecret)
	if ok && user == instance.MasterUsername && password == string(masterPassword) {
		return nil
	}
	if instance.EnableIAMDatabaseAuthentication && rdsValidateIAMAuthToken(instance, user, password) {
		return nil
	}
	return fmt.Errorf("password authentication failed for user %q", user)
}

func rdsValidateIAMAuthToken(instance RDSInstance, user, token string) bool {
	parsed, err := url.Parse("https://" + token)
	if err != nil || parsed.Host != net.JoinHostPort(instance.Endpoint, strconv.Itoa(instance.Port)) {
		return false
	}
	query := parsed.Query()
	if query.Get("Action") != "connect" || query.Get("DBUser") != user {
		return false
	}
	credential, ok := parseCredScope(query.Get("X-Amz-Credential"))
	if !ok || credential.service != "rds-db" || credential.region != awsRegion() {
		return false
	}
	expires, err := strconv.Atoi(query.Get("X-Amz-Expires"))
	if err != nil || expires <= 0 || expires > 900 {
		return false
	}
	signedAt, err := time.Parse("20060102T150405Z", query.Get("X-Amz-Date"))
	if err != nil || time.Now().Before(signedAt.Add(-5*time.Minute)) || time.Now().After(signedAt.Add(time.Duration(expires)*time.Second)) {
		return false
	}
	request := &http.Request{Method: http.MethodGet, URL: parsed, Host: parsed.Host, Header: make(http.Header)}
	request.Header.Set("X-Amz-Content-Sha256", rdsEmptyPayloadSHA256)
	result, signatureErr := sigv4VerifyPresigned(request, query, true)
	if signatureErr != nil || result != sigv4Verified {
		return false
	}
	request.Header.Set(
		"Authorization",
		fmt.Sprintf(
			"AWS4-HMAC-SHA256 Credential=%s, SignedHeaders=%s, Signature=%s",
			query.Get("X-Amz-Credential"),
			query.Get("X-Amz-SignedHeaders"),
			query.Get("X-Amz-Signature"),
		),
	)
	request.TLS = &tls.ConnectionState{}
	request.RemoteAddr = "127.0.0.1:0"
	resource := fmt.Sprintf(
		"arn:aws:rds-db:%s:%s:dbuser:%s/%s",
		awsRegion(), awsAccountID(), instance.DbiResourceId, user,
	)
	allowed, _, registered := iamAuthorize(request, "rds-db:connect", resource)
	return !registered || allowed
}

func rdsPostgreSQLAuthError(connection net.Conn, message string) {
	payload := []byte("SERROR\x00C28P01\x00M" + message + "\x00\x00")
	packet := make([]byte, 5+len(payload))
	packet[0] = 'E'
	binary.BigEndian.PutUint32(packet[1:5], uint32(len(payload)+4))
	copy(packet[5:], payload)
	_, _ = connection.Write(packet)
}

func rdsRelay(left, right net.Conn) {
	done := make(chan struct{}, 2)
	copySide := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		done <- struct{}{}
	}
	go copySide(left, right)
	go copySide(right, left)
	<-done
}

func rdsServerCertificate() (tls.Certificate, error) {
	rdsTLSOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			rdsTLSErr = err
			return
		}
		now := time.Now()
		template := &x509.Certificate{
			SerialNumber: big.NewInt(now.UnixNano()),
			Subject:      pkix.Name{CommonName: "Amazon RDS simulator"},
			NotBefore:    now.Add(-time.Hour),
			NotAfter:     now.AddDate(1, 0, 0),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if err != nil {
			rdsTLSErr = err
			return
		}
		keyDER := x509.MarshalPKCS1PrivateKey(key)
		rdsTLSCert, rdsTLSErr = tls.X509KeyPair(
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}),
		)
	})
	return rdsTLSCert, rdsTLSErr
}

func rdsStopDataPlane(instanceID string, deleteVolume bool) {
	value, ok := rdsDataPlanes.LoadAndDelete(instanceID)
	if !ok {
		return
	}
	runtime, ok := value.(*rdsDataPlaneRuntime)
	if !ok {
		return
	}
	_ = runtime.listener.Close()
	if runtime.handle != nil {
		runtime.handle.Cancel()
		_ = runtime.handle.Wait()
		if deleteVolume {
			_ = sim.RemoveVolume("sockerless-rds-" + runtime.instance.DBInstanceIdentifier)
		}
	}
}

// serveMySQL is implemented in rds_dataplane_mysql.go.
func (runtime *rdsDataPlaneRuntime) serveMySQL(client net.Conn) {
	rdsServeMySQL(runtime, client)
}
