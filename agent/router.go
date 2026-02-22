package agent

import (
	"encoding/base64"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Router dispatches incoming WebSocket messages to session handlers.
type Router struct {
	registry *SessionRegistry
	mp       *MainProcess // may be nil if not in keep-alive mode
	logger   zerolog.Logger
}

// NewRouter creates a new message router.
func NewRouter(registry *SessionRegistry, mp *MainProcess, logger zerolog.Logger) *Router {
	return &Router{
		registry: registry,
		mp:       mp,
		logger:   logger,
	}
}

// Handle processes a single incoming message.
func (rt *Router) Handle(msg *Message, conn *websocket.Conn, connMu *sync.Mutex) {
	switch msg.Type {
	case TypeExec:
		rt.handleExec(msg, conn, connMu)
	case TypeAttach:
		rt.handleAttach(msg, conn, connMu)
	case TypeStdin:
		rt.handleStdin(msg)
	case TypeCloseStdin:
		rt.handleCloseStdin(msg)
	case TypeSignal:
		rt.handleSignal(msg)
	case TypeResize:
		rt.handleResize(msg)
	default:
		rt.sendError(conn, connMu, msg.ID, "unknown message type: "+msg.Type)
	}
}

func (rt *Router) handleExec(msg *Message, conn *websocket.Conn, connMu *sync.Mutex) {
	if msg.ID == "" {
		rt.sendError(conn, connMu, "", "exec requires id")
		return
	}

	session, err := NewExecSession(msg.ID, msg, conn, connMu, rt.logger)
	if err != nil {
		rt.sendError(conn, connMu, msg.ID, err.Error())
		return
	}

	rt.registry.Register(session, conn)
	rt.logger.Debug().Str("id", msg.ID).Strs("cmd", msg.Cmd).Msg("exec session started")
}

func (rt *Router) handleAttach(msg *Message, conn *websocket.Conn, connMu *sync.Mutex) {
	if msg.ID == "" {
		rt.sendError(conn, connMu, "", "attach requires id")
		return
	}

	if rt.mp == nil {
		rt.sendError(conn, connMu, msg.ID, "no main process to attach to")
		return
	}

	session := NewAttachSession(msg.ID, rt.mp, conn, connMu, rt.logger)
	rt.registry.Register(session, conn)
	rt.logger.Debug().Str("id", msg.ID).Msg("attach session started")
}

func (rt *Router) handleStdin(msg *Message) {
	session, ok := rt.registry.Get(msg.ID)
	if !ok {
		rt.logger.Debug().Str("id", msg.ID).Msg("stdin for unknown session")
		return
	}

	data, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		rt.logger.Warn().Err(err).Str("id", msg.ID).Msg("failed to decode stdin data")
		return
	}

	if err := session.WriteStdin(data); err != nil {
		rt.logger.Debug().Err(err).Str("id", msg.ID).Msg("failed to write stdin")
	}
}

func (rt *Router) handleCloseStdin(msg *Message) {
	session, ok := rt.registry.Get(msg.ID)
	if !ok {
		return
	}
	_ = session.CloseStdin()
}

func (rt *Router) handleSignal(msg *Message) {
	session, ok := rt.registry.Get(msg.ID)
	if !ok {
		return
	}
	if err := session.Signal(msg.Signal); err != nil {
		rt.logger.Debug().Err(err).Str("id", msg.ID).Str("signal", msg.Signal).Msg("failed to send signal")
	}
}

func (rt *Router) handleResize(msg *Message) {
	session, ok := rt.registry.Get(msg.ID)
	if !ok {
		return
	}
	_ = session.Resize(msg.Width, msg.Height)
}

func (rt *Router) sendError(conn *websocket.Conn, connMu *sync.Mutex, id string, message string) {
	rt.logger.Warn().Str("id", id).Str("error", message).Msg("sending error to client")
	errMsg := Message{
		Type:    TypeError,
		ID:      id,
		Message: message,
	}
	connMu.Lock()
	defer connMu.Unlock()
	conn.WriteJSON(errMsg)
}
