package res

import (
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

type WsConn struct {
	id       int
	next     *websocket.Conn
	onClose  chan bool
	incoming chan []byte
	mu       sync.Mutex
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
	return w.WaitFor(value, 2*time.Second)
}

func (w *WsConn) WaitFor(value interface{}, duration time.Duration) error {
	logVerbose(w.id, errors.New("waitFor:"+duration.String()))

	select {
	case rd := <-w.incoming:
		return json.Unmarshal(rd, value)
	case <-time.After(duration):
		return errors.New("timeout")
	}
}

func (w *WsConn) Send(value interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	logVerbose(w.id, errors.New("ws: sending message"))

	if err := w.next.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}

	defer w.clearDeadlines()
	return w.next.WriteJSON(value)
}

func (w *WsConn) receive() {
	for {
		select {
		case <-w.onClose:
			return
		default:
		}

		logVerbose(w.id, errors.New("waiting for receive"))
		_, r, err := w.next.NextReader()
		if err != nil {
			logVerbose(w.id, fmt.Errorf("failed to read: %v", err))
			w.closeChan()
			return
		}

		buf, err := ioutil.ReadAll(r)
		if err != nil {
			logVerbose(w.id, fmt.Errorf("failed to read: %v", err))
			w.closeChan()
			return
		}

		logVerbose(w.id, errors.New("received message"))

		select {
		case w.incoming <- buf:
		case <-time.After(1 * time.Second):
			continue
		}
	}
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
			logVerbose(w.id, errors.New("websocket closed"))
			return
		case <-tc.C:
		}

		if err := w.ping(); err != nil {
			logVerbose(w.id, fmt.Errorf("ping failed: %v", err))
			return
		}
	}
}

func (w *WsConn) sendPing() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.next.SetWriteDeadline(time.Now().Add(time.Second))
	if err := w.next.WriteMessage(websocket.PingMessage, nil); err != nil {
		logVerbose(w.id, err)
		return err
	}

	return nil
}

func (w *WsConn) ping() error {

	logVerbose(w.id, errors.New("ws: sending ping"))
	pong := make(chan bool, 1)

	w.next.SetPongHandler(func(appData string) error {

		select {
		case pong <- true:
		case <-time.After(1 * time.Second):
		}

		return nil
	})

	if err := w.sendPing(); err != nil {
		return err
	}

	select {
	case <-pong:
		w.clearDeadlines()
		logVerbose(w.id, errors.New("ws: got 'pong'"))
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("ws: waited for pong but didn't get one")
	}
}

func (w *WsConn) clearDeadlines() {
	w.next.SetWriteDeadline(time.Now().Add(30 * time.Second))
}

func logVerbose(id int, err error) {
	if Verbose {
		log.Println("res: ", id, err.Error())
	}
}

type SocketHandler func(ctx context.Context, conn *WsConn)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var websocketIdCounter = 0
var muWebsocketIdCounter = &sync.Mutex{}

// WebSocket creates an endpoint to handle websocket upgrades. handler is responsible
// for processing and closing the connection.
func (rt *Router) WebSocket(method string, path string, handler SocketHandler) {

	rt.next.Methods(method).Path(path).HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		muWebsocketIdCounter.Lock()
		websocketIdCounter++
		id := websocketIdCounter
		muWebsocketIdCounter.Unlock()

		logVerbose(id, errors.New("got websocket request"))
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			logVerbose(id, fmt.Errorf("websocket upgrade failed: %v", err))
			return
		}

		logVerbose(id, errors.New("websocket request upgraded"))

		wConn := &WsConn{
			id:       id,
			next:     conn,
			incoming: make(chan []byte, 1),
			onClose:  make(chan bool),
			mu:       sync.Mutex{},
		}

		go wConn.receive()
		go wConn.watch()
		handler(request.Context(), wConn)
	})
}
