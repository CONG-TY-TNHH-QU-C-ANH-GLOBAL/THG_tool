package workspace

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	fiberws "github.com/gofiber/websocket/v2"
	serveragent "github.com/thg/scraper/internal/server/agent"
)

// perAccountVNCProxyHandler handles GET /ws/vnc/:id.
// It proxies the viewer WebSocket to the account container's x11vnc socket.
func (h *Handler) perAccountVNCProxyHandler() func(*fiberws.Conn) {
	return func(ws *fiberws.Conn) {
		accountID, err := strconv.ParseInt(ws.Params("id"), 10, 64)
		if err != nil {
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("invalid account id"))
			return
		}

		if h.workspace == nil {
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("workspace manager not initialized"))
			return
		}

		orgID, _ := ws.Locals("org_id").(int64)
		role, _ := ws.Locals("user_role").(string)
		acc, ok := serveragent.RequireAccountForOrgWS(h.db, orgID, role, accountID)
		if !ok {
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("access denied"))
			return
		}

		inst := h.workspaceInstanceForAccount(accountID, acc.Name)
		if inst == nil || inst.VNCPort == 0 {
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("browser not running; start it first"))
			return
		}

		vncAddr := fmt.Sprintf("127.0.0.1:%d", inst.VNCPort)
		log.Printf("[VNCProxy] Account %d -> %s", accountID, vncAddr)
		proxyVNC(ws, vncAddr)
	}
}

func proxyVNC(ws *fiberws.Conn, vncAddr string) {
	tcp, err := net.DialTimeout("tcp", vncAddr, 8*time.Second)
	if err != nil {
		log.Printf("[VNCProxy] Cannot reach VNC at %s: %v", vncAddr, err)
		_ = ws.WriteMessage(fiberws.TextMessage, []byte("VNC server not reachable; container may still be starting"))
		return
	}
	defer tcp.Close()

	log.Printf("[VNCProxy] Tunnel open: WebSocket <-> %s", vncAddr)
	errc := make(chan error, 2)

	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(fiberws.BinaryMessage, buf[:n]); werr != nil {
					errc <- werr
					return
				}
			}
			if err != nil {
				errc <- err
				return
			}
		}
	}()

	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if _, werr := tcp.Write(data); werr != nil {
				errc <- werr
				return
			}
		}
	}()

	<-errc
	log.Printf("[VNCProxy] Tunnel closed: %s", vncAddr)
}
