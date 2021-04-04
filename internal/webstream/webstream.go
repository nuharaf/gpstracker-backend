package webstream

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	gps "nuha.dev/gpstracker/internal/gps/server"
)

type WebstreamServer struct {
	db     *pgxpool.Pool
	server *http.Server
	logger zerolog.Logger
	gsrv   gps.ServerInterface
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

func NewWebstream(port int) *WebstreamServer {
	o := &WebstreamServer{}
	o.server = &http.Server{
		Addr:           ":3334",
		Handler:        o,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	o.logger = log.With().Str("module", "websocket").Logger()
	return o
}

func (ws *WebstreamServer) Run() {
	err := ws.server.ListenAndServe()
	if err != nil {
		panic(err)
	}
}

func (ws *WebstreamServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	defer c.Close(websocket.StatusInternalError, "the sky is falling")

	if err != nil {
		ws.logger.Err(err).Msg("Error while upgrading websocket")
		return
	}
	//read login info
	readCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, msg, err := c.Read(readCtx)
	if err != nil {
		ws.logger.Err(err).Msg("Error while reading auth token")
		return
	}
	//check token validity
	//subscribe list
	sublist := []string{}
	readCtx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = wsjson.Read(readCtx, c, sublist)
	if err != nil {
		ws.logger.Err(err).Msg("Error while reading subscription request")
		return
	}
	//check subscription permission
	//do subscribe
	wc := &WebstreamClient{srv: ws, c: c, tok: msg, wch: make(chan loc, 10), timer: time.NewTimer((10 * time.Second))}
	for _, v := range sublist {
		cs := ws.gsrv.GetClientState(v)
		cs.Sublist.Subscribe(wc)
	}
	wc.Run()
}

type loc struct {
	sender string
	data   []byte
}

type WebstreamClient struct {
	srv     *WebstreamServer
	c       *websocket.Conn
	wch     chan loc
	tok     []byte
	dropped uint64
	logger  zerolog.Logger
	timer   *time.Timer
	resetch chan struct{}
	closed  uint32
	err     error
}

func (wc *WebstreamClient) Run() {
	go wc.readloop()
	go wc.timeout_timer()

	for l := range wc.wch {
		err := wc.c.Write(context.Background(), websocket.MessageBinary, l.data)
		wc.logger.Err(err).Msg("Error while writing to connection")
	}
}

func (wc *WebstreamClient) timeout_timer() {
	for {
		select {
		case <-wc.timer.C:
			wc.c.Close(websocket.StatusAbnormalClosure, "timeout")
		case <-wc.resetch:
			if !wc.timer.Stop() {
				<-wc.timer.C
			}
			wc.timer.Reset(10 * time.Second)
		}
	}
}

func (wc *WebstreamClient) closeErr(err error) {
	atomic.StoreUint32(&wc.closed, 1)
	wc.err = err
}

func (wc *WebstreamClient) readloop() {
	for {
		_, _, err := wc.c.Read(context.Background())
		if err != nil {
			wc.logger.Err(err)
			wc.closeErr(err)
			return
		} else {
			wc.resetch <- struct{}{}
		}
	}
}

func (wc *WebstreamClient) Push(sender string, data []byte) error {
	s := loc{sender: sender, data: data}
	l := uint64(len(data))
	if wc.err != nil {
		return wc.err
	}
	select {
	case wc.wch <- s:
	default:
		atomic.AddUint64(&wc.dropped, l)
		wc.logger.Debug().Msgf("Dropping %d data", l)
	}
	return nil
}

func (wc *WebstreamClient) Closed() bool {
	return atomic.LoadUint32(&wc.closed) == 1
}

func (wc *WebstreamClient) Name() string {
	return "mocksub"
}
