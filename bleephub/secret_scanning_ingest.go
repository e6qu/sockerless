package bleephub

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitStorage "github.com/go-git/go-git/v5/storage"
)

type secretScanningPattern struct {
	patternID  string
	secretType string
	re         *regexp.Regexp
}

var secretScanningContentPatterns = []secretScanningPattern{
	{patternID: "ghp", secretType: "github_personal_access_token", re: regexp.MustCompile(`(?:ghp_[A-Za-z0-9_]{36,40}|github_pat_[A-Za-z0-9_]{40,128})`)},
	{patternID: "aws", secretType: "aws_access_key_id", re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{patternID: "google", secretType: "google_api_key", re: regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
	{patternID: "slack", secretType: "slack_incoming_webhook_url", re: regexp.MustCompile(`https://hooks\.slack\.com/services/[A-Za-z0-9_/-]+`)},
}

type secretScanningContentMatch struct {
	patternID  string
	secretType string
}

func (s *Server) scanCommitForSecretScanning(repo *Repo, stor gitStorage.Storer, commitHash plumbing.Hash, baseURL string) error {
	commit, err := object.GetCommit(stor, commitHash)
	if err != nil {
		return fmt.Errorf("load secret scanning commit %s: %w", commitHash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("load secret scanning tree %s: %w", commit.TreeHash, err)
	}
	files := tree.Files()
	defer files.Close()

	for {
		file, err := files.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("walk secret scanning tree %s: %w", commit.TreeHash, err)
		}
		reader, err := file.Reader()
		if err != nil {
			return fmt.Errorf("read secret scanning blob %s: %w", file.Hash, err)
		}
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return fmt.Errorf("read secret scanning blob %s: %w", file.Hash, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close secret scanning blob %s: %w", file.Hash, closeErr)
		}
		s.scanSecretScanningFile(repo, commitHash.String(), file.Name, file.Hash.String(), string(body), baseURL)
	}
	return nil
}

func (s *Server) scanSecretScanningFile(repo *Repo, commitSHA, path, blobSHA, body, baseURL string) {
	for _, pattern := range secretScanningContentPatterns {
		for _, match := range pattern.re.FindAllStringIndex(body, -1) {
			startLine, startColumn, endLine, endColumn := secretScanningMatchPosition(body, match[0], match[1])
			location := SecretScanningLocation{
				Type: "commit",
				Details: SecretScanningLocationDetails{
					Path:        path,
					StartLine:   startLine,
					EndLine:     endLine,
					StartColumn: startColumn,
					EndColumn:   endColumn,
					BlobSHA:     blobSHA,
					BlobURL:     fmt.Sprintf("%s/api/v3/repos/%s/git/blobs/%s", baseURL, repo.FullName, blobSHA),
					CommitSHA:   commitSHA,
					CommitURL:   fmt.Sprintf("%s/api/v3/repos/%s/commits/%s", baseURL, repo.FullName, commitSHA),
					HTMLURL:     fmt.Sprintf("%s/%s/blob/%s/%s#L%d", baseURL, repo.FullName, commitSHA, path, startLine),
				},
			}
			s.store.CreateSecretScanningAlertIfNew(repo.FullName, pattern.secretType, []SecretScanningLocation{location})
		}
	}
}

func secretScanningContentMatches(body string) []secretScanningContentMatch {
	seen := map[string]bool{}
	var out []secretScanningContentMatch
	for _, pattern := range secretScanningContentPatterns {
		if !pattern.re.MatchString(body) {
			continue
		}
		if seen[pattern.secretType] {
			continue
		}
		seen[pattern.secretType] = true
		out = append(out, secretScanningContentMatch{patternID: pattern.patternID, secretType: pattern.secretType})
	}
	return out
}

func (s *Server) secretScanningPushProtectionMatchesForCommit(stor gitStorage.Storer, commitHash plumbing.Hash) ([]secretScanningContentMatch, error) {
	commit, err := object.GetCommit(stor, commitHash)
	if err != nil {
		return nil, fmt.Errorf("load secret scanning push-protection commit %s: %w", commitHash, err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load secret scanning push-protection tree %s: %w", commit.TreeHash, err)
	}
	files := tree.Files()
	defer files.Close()

	seen := map[string]bool{}
	var out []secretScanningContentMatch
	for {
		file, err := files.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("walk secret scanning push-protection tree %s: %w", commit.TreeHash, err)
		}
		reader, err := file.Reader()
		if err != nil {
			return nil, fmt.Errorf("read secret scanning push-protection blob %s: %w", file.Hash, err)
		}
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read secret scanning push-protection blob %s: %w", file.Hash, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close secret scanning push-protection blob %s: %w", file.Hash, closeErr)
		}
		for _, match := range secretScanningContentMatches(string(body)) {
			if seen[match.secretType] {
				continue
			}
			seen[match.secretType] = true
			out = append(out, match)
		}
	}
	return out, nil
}

func (s *Server) createSecretScanningPushProtectionPlaceholder(repo *Repo, matches []secretScanningContentMatch) *SecretScanningPushProtectionPlaceholder {
	for _, match := range matches {
		if !s.store.SecretScanningPushProtectionEnabled(repo, match.patternID) {
			continue
		}
		if s.store.HasActiveSecretScanningPushProtectionBypass(repo.FullName, match.secretType, time.Now().UTC()) {
			continue
		}
		return s.store.CreateSecretScanningPushProtectionPlaceholder(repo.FullName, match.secretType)
	}
	return nil
}

func (s *Server) secretScanningPushProtectionPlaceholderForRef(repo *Repo, stor gitStorage.Storer, ref plumbing.ReferenceName, target plumbing.Hash) (*SecretScanningPushProtectionPlaceholder, error) {
	if !strings.HasPrefix(string(ref), "refs/heads/") {
		return nil, nil
	}
	if _, err := object.GetCommit(stor, target); err != nil {
		return nil, nil
	}
	matches, err := s.secretScanningPushProtectionMatchesForCommit(stor, target)
	if err != nil {
		return nil, err
	}
	return s.createSecretScanningPushProtectionPlaceholder(repo, matches), nil
}

func secretScanningMatchPosition(body string, start, end int) (startLine, startColumn, endLine, endColumn int) {
	startLine, startColumn = secretScanningOffsetPosition(body, start)
	endLine, endColumn = secretScanningOffsetPosition(body, end)
	return startLine, startColumn, endLine, endColumn
}

func secretScanningOffsetPosition(body string, offset int) (line, column int) {
	line = 1
	lastLineStart := 0
	for {
		next := strings.IndexByte(body[lastLineStart:offset], '\n')
		if next < 0 {
			break
		}
		line++
		lastLineStart += next + 1
	}
	return line, offset - lastLineStart
}
