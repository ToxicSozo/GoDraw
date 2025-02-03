package wsserver

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

type WSServer interface {
	Start() error
}

type wsSrv struct {
	mux   *http.ServeMux
	srv   *http.Server
	wsUpg websocket.Upgrader
}

func NewWsServer(addr string) WSServer {
	m := http.NewServeMux()
	return &wsSrv{
		mux: m,
		srv: &http.Server{
			Addr:    addr,
			Handler: m,
		},
		wsUpg: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (ws *wsSrv) Start() error {
	log.Printf("Starting WebSocket server on %s", ws.srv.Addr)
	ws.mux.HandleFunc("/ws", ws.wsHandler)
	ws.mux.HandleFunc("/test", ws.testHandler)
	return ws.srv.ListenAndServe()
}

func (ws *wsSrv) testHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Test handler called")
	w.Write([]byte("Test is successful"))
}

func (ws *wsSrv) wsHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("WebSocket upgrade request received")

	conn, err := ws.wsUpg.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		http.Error(w, "Could not upgrade connection", http.StatusBadRequest)
		return
	}

	log.Println("WebSocket connection established")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message from client: %v", err)
			break
		}

		log.Printf("Received message: %s", message)

		if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("Error writing message to client: %v", err)
			break
		}
	}

	log.Println("WebSocket connection closed")
}
