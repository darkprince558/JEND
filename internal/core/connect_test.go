package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/darkprince558/jend/internal/transport"
	"github.com/stretchr/testify/assert"
)

func TestConnectSession_Real(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	code := "test-connect-code"
	port := "9555"

	// Mock file creation
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("Hello from the client test file"), 0644)
	assert.NoError(t, err)

	tr := transport.NewQUICTransport()

	// 1. Setup Host Listener
	l, err := tr.Listen(port)
	assert.NoError(t, err)
	defer l.Close()

	hostReady := make(chan bool)
	hostReceived := make(chan bool, 2)
	clientReceived := make(chan bool, 1)

	// Host Goroutine
	go func() {
		hostReady <- true
		conn, err := l.Accept(ctx)
		if !assert.NoError(t, err) {
			return
		}

		s, err := conn.AcceptStream(ctx)
		if !assert.NoError(t, err) {
			return
		}

		key, err := PerformPAKE(s, code, 0)
		if !assert.NoError(t, err) {
			return
		}

		session := &ConnectSession{
			conn: conn,
			key:  key,
			ctx:  ctx,
			sendMsg: func(msg tea.Msg) {
				// We expect to receive a text and a file
				hostReceived <- true
			},
		}

		go session.acceptLoop()

		// Send a text message to client
		time.Sleep(500 * time.Millisecond)
		err = session.SendMessage("Welcome to the host", false)
		assert.NoError(t, err)
	}()

	<-hostReady

	// Client Goroutine
	conn, err := tr.Dial("127.0.0.1:" + port)
	assert.NoError(t, err)

	s, err := conn.OpenStreamSync(ctx)
	assert.NoError(t, err)

	key, err := PerformPAKE(s, code, 1)
	assert.NoError(t, err)

	clientSession := &ConnectSession{
		conn: conn,
		key:  key,
		ctx:  ctx,
		sendMsg: func(msg tea.Msg) {
			clientReceived <- true
		},
	}

	go clientSession.acceptLoop()

	// Client sends text and file
	err = clientSession.SendMessage("Hello from client", false)
	assert.NoError(t, err)

	err = clientSession.SendMessage(testFile, true)
	assert.NoError(t, err)

	// Wait for assertions
	select {
	case <-hostReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("Host timeout waiting for msg 1")
	}
	select {
	case <-hostReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("Host timeout waiting for msg 2")
	}

	select {
	case <-clientReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("Client timeout waiting for text msg")
	}

	conn.CloseWithError(0, "done")
}
