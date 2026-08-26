package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/protocol"
)

const (
	defaultRequestTimeout = 30 * time.Second
	invalidRequestExit    = 2
	internalErrorExit     = 10
)

type Router interface {
	Route(context.Context, protocol.Request) protocol.Reply
}

type Server struct {
	socketPath     string
	router         Router
	logger         *log.Logger
	requestTimeout time.Duration
}

func New(socketPath string, router Router, logger *log.Logger) *Server {
	return &Server{
		socketPath:     socketPath,
		router:         router,
		logger:         logger,
		requestTimeout: defaultRequestTimeout,
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	if err := prepareSocket(s.socketPath); err != nil {
		return err
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on Unix socket: %w", err)
	}

	defer func() {
		_ = listener.Close()
		_ = os.Remove(s.socketPath)
	}()

	if err := os.Chmod(s.socketPath, 0660); err != nil {
		return fmt.Errorf("set Unix socket permissions: %w", err)
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil || errors.Is(acceptErr, net.ErrClosed) {
				return nil
			}

			return fmt.Errorf("accept Unix connection: %w", acceptErr)
		}

		go s.handleConnection(ctx, connection)
	}
}

func (s *Server) handleConnection(parent context.Context, connection net.Conn) {
	defer connection.Close()

	ctx, cancel := context.WithTimeout(parent, s.requestTimeout)
	defer cancel()

	_ = connection.SetDeadline(time.Now().Add(s.requestTimeout))

	request, err := decodeRequest(connection)
	if err != nil {
		s.logger.Printf("invalid backend request: %v", err)
		s.writeReply(connection, protocol.Reply{
			ExitCode: invalidRequestExit,
			Response: api.Failure(
				"INVALID_REQUEST",
				"La requête transmise au backend Go est invalide.",
			),
		})
		return
	}

	reply := s.router.Route(ctx, request)
	s.writeReply(connection, reply)
}

func (s *Server) writeReply(writer io.Writer, reply protocol.Reply) {
	if err := json.NewEncoder(writer).Encode(reply); err != nil {
		s.logger.Printf("write backend response: %v", err)
	}
}

func decodeRequest(reader io.Reader) (protocol.Request, error) {
	limited := io.LimitReader(reader, protocol.MaximumMessageBytes+1)
	buffered := bufio.NewReader(limited)
	message, err := buffered.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return protocol.Request{}, err
	}
	if int64(len(message)) > protocol.MaximumMessageBytes {
		return protocol.Request{}, errors.New("request exceeds maximum size")
	}
	if len(bytes.TrimSpace(message)) == 0 {
		return protocol.Request{}, errors.New("request is empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.DisallowUnknownFields()

	var request protocol.Request
	if err = decoder.Decode(&request); err != nil {
		return protocol.Request{}, err
	}

	if decoder.Decode(&struct{}{}) != io.EOF {
		return protocol.Request{}, errors.New("request contains trailing data")
	}

	return request, nil
}

func prepareSocket(socketPath string) error {
	if !filepath.IsAbs(socketPath) {
		return errors.New("Unix socket path must be absolute")
	}

	if err := os.MkdirAll(filepath.Dir(socketPath), 0750); err != nil {
		return fmt.Errorf("create Unix socket directory: %w", err)
	}

	info, err := os.Lstat(socketPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing Unix socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("refusing to replace a non-socket path")
	}

	if err := os.Remove(socketPath); err != nil {
		return fmt.Errorf("remove stale Unix socket: %w", err)
	}

	return nil
}

func InternalError() protocol.Reply {
	return protocol.Reply{
		ExitCode: internalErrorExit,
		Response: api.Failure(
			"INTERNAL_ERROR",
			"Le backend Go a rencontré une erreur interne.",
		),
	}
}
