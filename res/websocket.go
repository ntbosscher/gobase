package res

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"sync"
	"time"
)

type WsConn struct {
	next    *websocket.Conn
	onClose chan bool
	mu      sync.Mutex
}

func (w *WsConn) OnClose() <-chan bool {
	return w.onClose
}

func (w *WsConn) Close() error {
	if err := w.next.Close(); err != nil {
		return err
	}

	w.closeChan()
	return nil
}

func (w *WsConn) closeChan() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.onClose != nil {
		close(w.onClose)
		w.onClose = nil
	}
}

func (w *WsConn) Read(value interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.next.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}

	return w.next.ReadJSON(value)
}

func (w *WsConn) Send(value interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.next.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}

	return w.next.WriteJSON(value)
}

func (w *WsConn) watch() {

	w.next.SetCloseHandler(func(code int, text string) error {
		w.closeChan()
		return nil
	})

	defer w.Close()
	tc := time.NewTicker(5 * time.Second)
	defer tc.Stop()

	for {
		select {
		case <-w.onClose:
			return
		case <-tc.C:
		}

		if err := w.ping(); err != nil {
			logVerbose(err)
			return
		}
	}
}

func (w *WsConn) ping() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.next.SetWriteDeadline(time.Now().Add(time.Second))
	if err := w.next.WriteMessage(websocket.PingMessage, nil); err != nil {
		return err
	}

	w.next.SetReadDeadline(time.Now().Add(time.Second))
	t, _, err := w.next.ReadMessage()
	if err != nil {
		return err
	}

	if t != websocket.PongMessage {
		return errors.New("invalid ping response message type")
	}

	return nil
}

func logVerbose(err error) {
	if Verbose {
		log.Println("res: " + err.Error())
	}
}

type SocketHandler func(ctx context.Context, conn *WsConn)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WebSocket creates an endpoint to handle websocket upgrades. handler is responsible
// for processing and closing the connection.
func (rt *Router) WebSocket(method string, path string, handler SocketHandler) {
	rt.next.Methods(method).Path(path).HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			log.Println("websocket upgrade failed:", err)
			return
		}

		wConn := &WsConn{
			next:    conn,
			onClose: make(chan bool),
			mu:      sync.Mutex{},
		}

		go wConn.watch()
		handler(request.Context(), wConn)
	})
}
