package bleephub

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestGitSSHTransportPushAndClone(t *testing.T) {
	t.Helper()
	admin := testServer.store.LookupUserByLogin("admin")
	name := "ssh-transport-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	testServer.store.CreateRepo(admin, name, "", true)

	public, err := ssh.NewPublicKey(testSSHKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{"title": "transport test", "key": strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public)))})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/api/v3/user/keys", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create SSH key status = %d", resp.StatusCode)
	}

	keyBlock, err := ssh.MarshalPrivateKey(testSSHKey, "bleephub transport test")
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	keyPath := filepath.Join(temp, "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(keyBlock), 0o600); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(temp, "worktree")
	runGit := func(dir string, args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -i "+keyPath+" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null")
		if output, runErr := command.CombinedOutput(); runErr != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), runErr, output)
		}
	}
	url := "ssh://git@" + testSSHAddr + "/admin/" + name + ".git"
	emptyClone := filepath.Join(temp, "empty-clone")
	runGit(temp, "clone", url, emptyClone)
	if _, err := os.Stat(filepath.Join(emptyClone, ".git")); err != nil {
		t.Fatalf("empty cloned repository did not contain Git metadata: %v", err)
	}
	httpEmptyClone := filepath.Join(temp, "http-empty-clone")
	httpURL := strings.Replace(testBaseURL, "://", "://admin:"+defaultToken+"@", 1) + "/admin/" + name + ".git"
	runGit(temp, "clone", httpURL, httpEmptyClone)
	if _, err := os.Stat(filepath.Join(httpEmptyClone, ".git")); err != nil {
		t.Fatalf("empty HTTP cloned repository did not contain Git metadata: %v", err)
	}

	runGit(temp, "init", worktree)
	runGit(worktree, "config", "user.name", "Bleephub SSH test")
	runGit(worktree, "config", "user.email", "ssh-test@bleephub.invalid")
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("SSH transport\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(worktree, "add", "README.md")
	runGit(worktree, "commit", "-m", "SSH transport")
	runGit(worktree, "remote", "add", "origin", url)
	runGit(worktree, "push", "origin", "HEAD:main")
	clone := filepath.Join(temp, "clone")
	runGit(temp, "clone", url, clone)
	if _, err := os.Stat(filepath.Join(clone, "README.md")); err != nil {
		t.Fatalf("cloned repository did not contain pushed file: %v", err)
	}
}

func TestSSHGITURL(t *testing.T) {
	t.Setenv("BLEEPHUB_SSH_HOST", "ssh.bleephub.example")
	if got, want := sshGitURL("owner/repository"), "git@ssh.bleephub.example:owner/repository.git"; got != want {
		t.Fatalf("sshGitURL default port = %q, want %q", got, want)
	}
	t.Setenv("BLEEPHUB_SSH_HOST", "127.0.0.1:2222")
	if got, want := sshGitURL("owner/repository"), "ssh://git@127.0.0.1:2222/owner/repository.git"; got != want {
		t.Fatalf("sshGitURL custom port = %q, want %q", got, want)
	}
}
