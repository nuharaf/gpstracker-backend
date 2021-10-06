package webstream

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nhooyr.io/websocket"
	"nuha.dev/gpstracker/internal/gps/client"
	gps "nuha.dev/gpstracker/internal/gps/serverimpl"
)

type WebstreamServer struct {
	server *http.Server
	logger zerolog.Logger
	gsrv   *gps.Server
	config WebStreamConfig
	db     *pgxpool.Pool
}

type WebStreamConfig struct {
	MockToken  bool
	ListenAddr string
}

type WsSubscriber struct {
	loc     chan []byte
	skipped uint64
	pushed  uint64
}

const (
	CLogin string = "login"
	CSub   string = "subscribe"
	CUnsub string = "unsubscribe"
	CLimit string = "limit"
)

func (wsub *WsSubscriber) Push(d []byte) {
	select {
	case wsub.loc <- d:
		atomic.AddUint64(&wsub.pushed, 1)
	default:
		atomic.AddUint64(&wsub.skipped, 1)
	}
}

func NewWebstream(db *pgxpool.Pool, gps_server *gps.Server, config WebStreamConfig) *WebstreamServer {
	o := &WebstreamServer{config: config}
	o.server = &http.Server{
		Addr:           config.ListenAddr,
		Handler:        http.HandlerFunc(o.serve_http),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	o.logger = log.With().Str("module", "websocket").Logger()
	o.gsrv = gps_server
	o.db = db
	return o
}

func (ws *WebstreamServer) Run() {
	err := ws.server.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func (ws *WebstreamServer) serve_http(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled,
	})
	defer c.Close(websocket.StatusInternalError, "unhandled error")

	if err != nil {
		ws.logger.Err(err).Msg("Error while upgrading websocket")
		return
	}
	//read login info
	readCtx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()
	_, msg, err := c.Read(readCtx)
	if err != nil {
		ws.logger.Err(err).Msg("Error while reading auth token")
		return
	}
	ws.logger.Info().Msg("websocket token received")

	var user_id, session_id string
	var valid bool
	if !ws.config.MockToken {
		valid, user_id, session_id = ws.validate_token(r.Context(), string(msg))
		if !valid {
			c.Close(websocket.StatusPolicyViolation, "invalid token")
		}
	}
	wc := &WebstreamClient{uid: user_id, sid: session_id, srv: ws, c: c, tok: msg, logger: ws.logger, lock: sync.Mutex{}, buf: make([][]byte, 0, 10), wg: sync.WaitGroup{}}
	go wc.writeLoop()
	go wc.readloop()
	wc.wg.Wait()
}

func (ws *WebstreamServer) validate_token(ctx context.Context, token string) (bool, string, string) {
	var user_id, session_id string
	var valid_until time.Time
	row := ws.db.QueryRow(ctx, `SELECT "user".id,session.session_id
	FROM "user" inner join session ON session.user_id = "user".id 
	WHERE session.ws_token = $1 
	AND "user".init_done = TRUE 
	AND "user".suspended = FALSE
	AND session.valid_until > NOW()`, token)
	err := row.Scan(&user_id, &session_id, &valid_until)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, "", ""
		} else {
			panic(err)
		}
	} else {
		return true, user_id, session_id
	}
}

type WebstreamClient struct {
	lock    sync.Mutex
	wg      sync.WaitGroup
	srv     *WebstreamServer
	c       *websocket.Conn
	uid     string
	sid     string
	tok     []byte
	logger  zerolog.Logger
	closed  bool
	err     error
	buf     [][]byte
	sublist map[uint64]*client.ClientState
}

func (wc *WebstreamClient) closeErr(err error) {
	wc.closed = true
	wc.err = err
}

func (wc *WebstreamClient) readloop() {
	wc.wg.Add(1)
	defer wc.wg.Done()
	for {
		_, msg, err := wc.c.Read(context.Background())
		fmt.Print(string(msg))
		if err != nil {
			wc.logger.Err(err)
			wc.lock.Lock()
			wc.closeErr(err)
			wc.lock.Unlock()
			return
		} else {
			if string(msg[:6]) == "ADDSUB" {
				subname := strings.Split(string(msg[7:]), ",")
				wc.logger.Debug().Strs("addsub", subname).Msg("receive add subscription message")
				for _, v := range subname {
					id, err := strconv.ParseUint(v, 10, 64)
					if err != nil {
						gpsclient := wc.srv.gsrv.GetGpsClientState(id)
						gpsclient.Sublist.Subscribe(wc)
						wc.sublist[id] = gpsclient
					}

				}
			} else if string(msg[:6]) == "DELSUB" {
				subname := strings.Split(string(msg[7:]), ",")
				wc.logger.Debug().Strs("delsub", subname).Msg("receive delete subscription message")
				for _, v := range subname {
					id, err := strconv.ParseUint(v, 10, 64)
					if err != nil {
						gpsclient, ok := wc.sublist[id]
						if ok {
							gpsclient.Sublist.Unsubscribe(wc)
							delete(wc.sublist, id)
						}
					}
				}
			}
		}
	}
}

func (wc *WebstreamClient) writeLoop() {
	wc.wg.Add(1)
	defer wc.wg.Done()
	for {
		wc.lock.Lock()
		l := len(wc.buf)
		for _, d := range wc.buf {
			err := wc.c.Write(context.Background(), websocket.MessageBinary, d)
			if err != nil {
				wc.logger.Err(err).Msg("Error while writing to connection")
				wc.closeErr(err)
				wc.lock.Unlock()
				return
			}
		}
		wc.buf = wc.buf[:0]
		wc.lock.Unlock()
		if l == 0 {
			time.Sleep(5 * time.Second)
		} else {
			time.Sleep(time.Second)
		}
	}
}

func (wc *WebstreamClient) Push(sender uint64, data []byte) bool {
	wc.lock.Lock()
	if wc.closed {
		return true
	}
	wc.buf = append(wc.buf, data)
	wc.lock.Unlock()
	return false
}

// func (wc *WebstreamClient) Name() string {
// 	return "mocksub"
// }
