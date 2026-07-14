package bleephub

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
	"golang.org/x/crypto/ssh"
)

// startGitSSH serves the standard SSH Git transport when BLEEPHUB_SSH_ADDR
// and BLEEPHUB_SSH_HOST_KEY are configured. The host key is required so a
// restarted durable server retains its SSH host identity instead of silently
// generating a different key for every process.
func (s *Server) startGitSSH() error {
	addr := strings.TrimSpace(os.Getenv("BLEEPHUB_SSH_ADDR"))
	if addr == "" {
		return nil
	}
	hostKey := os.Getenv("BLEEPHUB_SSH_HOST_KEY")
	if hostKey == "" {
		return errors.New("BLEEPHUB_SSH_ADDR is configured but BLEEPHUB_SSH_HOST_KEY is empty")
	}
	signer, err := ssh.ParsePrivateKey([]byte(hostKey))
	if err != nil {
		return fmt.Errorf("parse BLEEPHUB_SSH_HOST_KEY: %w", err)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen for SSH Git transport on %s: %w", addr, err)
	}
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			user := s.store.LookupUserBySSHKey(key)
			if user == nil || user.Suspended {
				return nil, errors.New("unknown SSH key")
			}
			return &ssh.Permissions{Extensions: map[string]string{"bleephub-user-id": fmt.Sprintf("%d", user.ID)}}, nil
		},
	}
	config.AddHostKey(signer)
	s.logger.Info().Str("addr", addr).Msg("bleephub SSH Git transport listening")
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				s.logger.Error().Err(acceptErr).Msg("SSH Git transport listener stopped")
				return
			}
			go s.serveGitSSHConn(conn, config)
		}
	}()
	return nil
}

func (s *Server) serveGitSSHConn(conn net.Conn, config *ssh.ServerConfig) {
	defer func() { _ = conn.Close() }()
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		s.logger.Debug().Err(err).Msg("SSH Git handshake rejected")
		return
	}
	defer func() { _ = serverConn.Close() }()
	go ssh.DiscardRequests(requests)
	userID := serverConn.Permissions.Extensions["bleephub-user-id"]
	user := s.store.GetUserByID(mustAtoi(userID))
	if user == nil {
		return
	}
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only SSH session channels are supported")
			continue
		}
		channel, requests, openErr := newChannel.Accept()
		if openErr != nil {
			continue
		}
		go s.serveGitSSHSession(channel, requests, user)
	}
}

func mustAtoi(value string) int {
	var result int
	_, _ = fmt.Sscanf(value, "%d", &result)
	return result
}

func (s *Server) serveGitSSHSession(channel ssh.Channel, requests <-chan *ssh.Request, user *User) {
	defer func() { _ = channel.Close() }()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			_ = request.Reply(false, nil)
			return
		}
		service, owner, repoName, ok := parseGitSSHCommand(payload.Command)
		if !ok {
			_, _ = io.WriteString(channel.Stderr(), "Unsupported SSH command.\n")
			_ = request.Reply(false, nil)
			return
		}
		_ = request.Reply(true, nil)
		exitStatus := uint32(0)
		if err := s.runGitSSHService(channel, service, owner, repoName, user); err != nil {
			s.logger.Debug().Err(err).Str("service", service).Str("repo", owner+"/"+repoName).Msg("SSH Git request failed")
			_, _ = io.WriteString(channel.Stderr(), "Bleephub: "+err.Error()+"\n")
			exitStatus = 1
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitStatus}))
		return
	}
}

func parseGitSSHCommand(command string) (service, owner, repo string, ok bool) {
	fields := strings.Fields(command)
	if len(fields) != 2 || (fields[0] != "git-upload-pack" && fields[0] != "git-receive-pack") {
		return "", "", "", false
	}
	path := strings.Trim(fields[1], "'\"")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	owner, repo = splitRepoPath("/" + path)
	if owner == "" || repo == "" || strings.ContainsAny(path, "\\\n\r") {
		return "", "", "", false
	}
	return fields[0], owner, repo, true
}

func sshGitURL(fullName string) string {
	host := strings.TrimSpace(os.Getenv("BLEEPHUB_SSH_HOST"))
	if host == "" {
		return ""
	}
	// SCP-style Git URLs cannot encode a non-default SSH port. A configured
	// host-and-port therefore uses the standard SSH URL form instead.
	if _, _, err := net.SplitHostPort(host); err == nil {
		return "ssh://git@" + host + "/" + fullName + ".git"
	}
	return "git@" + host + ":" + fullName + ".git"
}

// sshChannelReader deliberately does not implement io.Closer. go-git's
// receive-pack decoder retains the supplied reader as its packfile and closes
// it after ingestion; closing an SSH channel there would also close its output
// side before the required report-status response can be written.
type sshChannelReader struct{ io.Reader }

func (s *Server) runGitSSHService(channel ssh.Channel, service, owner, repoName string, user *User) error {
	repo := s.store.GetRepo(owner, repoName)
	if repo == nil || s.resolveGitRepo(owner, repoName) == nil {
		return transport.ErrRepositoryNotFound
	}
	if service == "git-upload-pack" && !canReadRepo(s.store, user, repo) {
		return errors.New("repository access denied")
	}
	if service == "git-receive-pack" && !canPushRepo(s.store, user, repo) {
		return errors.New("repository write access denied")
	}
	ep, err := transport.NewEndpoint(fmt.Sprintf("/%s/%s", owner, repoName))
	if err != nil {
		return err
	}
	server := gitserver.NewServer(&storeLoader{store: s.store})
	if service == "git-upload-pack" {
		session, err := server.NewUploadPackSession(ep, nil)
		if err != nil {
			return err
		}
		info, err := session.AdvertisedReferencesContext(context.Background())
		if err != nil && !errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return err
		}
		if info != nil {
			if err := info.Encode(channel); err != nil {
				return err
			}
		} else if err := pktline.NewEncoder(channel).Flush(); err != nil {
			return err
		}
		requestReader := bufio.NewReader(channel)
		empty, err := flushOnlyGitRequest(requestReader)
		if err != nil {
			return err
		}
		if empty {
			return pktline.NewEncoder(channel).Flush()
		}
		request := packp.NewUploadPackRequest()
		if err := request.Decode(requestReader); err != nil {
			return err
		}
		response, err := session.UploadPack(context.Background(), request)
		if err != nil {
			return err
		}
		return response.Encode(channel)
	}
	session, err := server.NewReceivePackSession(ep, nil)
	if err != nil {
		return err
	}
	info, err := session.AdvertisedReferencesContext(context.Background())
	if err != nil && !errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return err
	}
	if info != nil {
		if err := info.Encode(channel); err != nil {
			return err
		}
	} else if err := pktline.NewEncoder(channel).Flush(); err != nil {
		return err
	}
	request := packp.NewReferenceUpdateRequest()
	if err := request.Decode(sshChannelReader{Reader: channel}); err != nil {
		return err
	}
	result, err := session.ReceivePack(context.Background(), request)
	if err != nil && !strings.Contains(err.Error(), "EOF") {
		return err
	}
	s.afterGitReceivePack(repo, user, request, s.externalURL)
	if result != nil {
		return result.Encode(channel)
	}
	return nil
}

// flushOnlyGitRequest recognizes the valid upload-pack request a Git client
// sends after advertising an empty repository. There are no objects to want,
// so the request consists solely of the pkt-line flush marker. go-git's
// request decoder intentionally expects at least one want line and cannot
// represent this protocol-valid case.
func flushOnlyGitRequest(reader *bufio.Reader) (bool, error) {
	header, err := reader.Peek(4)
	if err != nil {
		return false, err
	}
	return string(header) == "0000", nil
}
