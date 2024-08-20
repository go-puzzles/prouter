package prouter

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-puzzles/plog"
	"github.com/gorilla/mux"
)

const (
	DebugMode = iota
	ReleaseMode
)

var (
	prouterMode = DebugMode
)

func SetMode(value int64) {
	switch value {
	case DebugMode:
		prouterMode = DebugMode
	case ReleaseMode:
		prouterMode = ReleaseMode
	default:
		panic("Prouter mode unknown: " + strconv.FormatInt(value, 10) + " (available mode: debug release test)")
	}
}

type Prouter struct {
	RouterGroup
	host        string
	scheme      string
	middlewares []Middleware
}

type RouterOption func(v *Prouter)

func WithHost(host string) RouterOption {
	return func(v *Prouter) {
		v.host = host
	}
}

func WithScheme(scheme string) RouterOption {
	return func(v *Prouter) {
		v.scheme = scheme
	}
}

func WithNotFoundHandler(handler http.Handler) RouterOption {
	return func(v *Prouter) {
		v.router.NotFoundHandler = handler
	}
}

func WithMethodNotAllowedHandler(handler http.Handler) RouterOption {
	return func(v *Prouter) {
		v.router.MethodNotAllowedHandler = handler
	}
}

func (v *Prouter) parseOptions(opts ...RouterOption) {
	for _, opt := range opts {
		opt(v)
	}
	if v.host != "" {
		v.router = v.router.Host(v.host).Subrouter()
	}

	if v.scheme != "" {
		v.router = v.router.Schemes(v.host).Subrouter()
	}
}

func New(opts ...RouterOption) *Prouter {
	m := mux.NewRouter()

	v := &Prouter{
		RouterGroup: newGroupWithRouter(m),
		middlewares: make([]Middleware, 0),
	}
	v.RouterGroup.root = true
	v.RouterGroup.prouter = v
	v.parseOptions(opts...)

	return v
}

func NewProuter(opts ...RouterOption) *Prouter {
	v := New(opts...)
	v.UseMiddleware(
		NewLogMiddleware(),
		NewRecoveryMiddleware(),
	)

	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = WriteJSON(w, http.StatusNotFound, ErrorResponse(http.StatusNotFound, "page not found"))
	})
	v.router.NotFoundHandler = notFoundHandler
	v.router.MethodNotAllowedHandler = notFoundHandler
	return v
}
func (v *Prouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	v.router.ServeHTTP(w, r)
}

func (v *Prouter) ServeHandler() *mux.Router {
	return v.router
}

func (v *Prouter) Run(addr string) error {
	srv := http.Server{
		Addr:    addr,
		Handler: v,
	}
	return srv.ListenAndServe()
}

func (v *Prouter) initRouter(r iRoute) {
	f := v.makeHttpHandler(r)

	vr := r.router.Path(r.Path())
	if r.Method() != "" {
		vr = vr.Methods(r.Method())
	}

	if r.routeOption != nil {
		vr = r.routeOption(vr)
	}

	mr := vr.Handler(f)
	v.debugPrintRoute(r.Method(), mr, r.Handler())
}

func (v *Prouter) UseMiddleware(m ...Middleware) {
	v.middlewares = append(v.middlewares, m...)
}

func (v *Prouter) handleGlobalMiddleware(handler handlerFunc) handlerFunc {
	h := handler

	for _, m := range slices.Backward(v.middlewares) {
		h = m.WrapHandler(h)
	}

	return h
}

func (v *Prouter) handelrName(handler handlerFunc) string {
	funcName := plog.GetFuncName(handler)
	fs := strings.Split(funcName, ".")

	return fs[len(fs)-1]
}

func remoteIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return ""
	}
	return ip
}

func clientIP(r *http.Request) string {
	remoteIP := net.ParseIP(remoteIP(r))
	if remoteIP == nil {
		return ""
	}

	return remoteIP.String()
}

func (v *Prouter) makeHttpHandler(wr iRoute) http.HandlerFunc {
	c := plog.With(context.Background(), "handler", wr.Handler().Name())

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		raw := r.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}

		ctx := &Context{
			Context:   c,
			Request:   r,
			Writer:    w,
			Path:      path,
			Method:    r.Method,
			ClientIp:  clientIP(r),
			startTime: time.Now(),
		}
		r = r.WithContext(ctx)

		vars := mux.Vars(r)
		if vars == nil {
			vars = make(map[string]string)
		}
		ctx.vars = vars
		ctx.router = v

		handlerFunc := v.handleGlobalMiddleware(wr.Handler())
		handlerFunc = wr.handleSpecifyMiddleware(handlerFunc)

		status, resp := v.packResponseTmpl(handlerFunc.Handle(ctx))

		_ = WriteJSON(w, status, resp)
	}
}

func (v *Prouter) packResponseTmpl(resp Response, err error) (status int, ret ResponseTmpl) {
	var (
		code int
		data any
		msg  string
	)

	data = func() any {
		if resp == nil {
			return nil
		}

		return resp.GetData()
	}()

	code = func() int {
		if resp == nil {
			return 200
		}
		return resp.GetCode()
	}()

	if err != nil {
		if resp == nil || resp.GetMessage() == "" {
			msg = err.Error()
		} else {
			msg = resp.GetMessage()
		}

		if code == 0 || code == 200 {
			code = http.StatusInternalServerError
		}
	}

	ret = NewResponseTmpl()
	ret.SetCode(code)
	ret.SetMessage(msg)
	ret.SetData(data)

	return code, ret
}

func (v *Prouter) debugPrintRoute(method string, route *mux.Route, handler handlerFunc) {
	if prouterMode != DebugMode {
		return
	}
	if method == "" {
		method = "ANY"
	}

	handlerName := handler.Name()
	url, err := route.GetPathTemplate()
	if err != nil {
		plog.Errorf("get handler: %v iRoute url error: %v", handlerName, err)
	}
	plog.Infof("Method: %-6s Router: %-26s Handler: %s", method, url, handlerName)
}
