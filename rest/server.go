package rest

import (
	"crypto/tls"
	"log"
	"net/http"
	"path"

	"github.com/tal-tech/go-zero/core/logx"
	"github.com/tal-tech/go-zero/rest/handler"
	"github.com/tal-tech/go-zero/rest/httpx"
	"github.com/tal-tech/go-zero/rest/router"
)

type (
	runOptions struct {
		start func(*engine) error
	}

	// RunOption defines the method to customize a Server.
	RunOption func(*Server)

	// A Server is a http server.
	Server struct {
		ngin *engine
		opts runOptions
	}
)

// MustNewServer returns a server with given config of c and options defined in opts.
// Be aware that later RunOption might overwrite previous one that write the same option.
// The process will exit if error occurs.
func MustNewServer(c RestConf, opts ...RunOption) *Server {
	server, err := NewServer(c, opts...)
	if err != nil {
		log.Fatal(err)
	}

	return server
}

// NewServer returns a server with given config of c and options defined in opts.
// Be aware that later RunOption might overwrite previous one that write the same option.
func NewServer(c RestConf, opts ...RunOption) (*Server, error) {
	if err := c.SetUp(); err != nil {
		return nil, err
	}

	server := &Server{
		ngin: newEngine(c),
		opts: runOptions{
			start: func(ng *engine) error {
				return ng.Start()
			},
		},
	}

	for _, opt := range opts {
		opt(server)
	}

	return server, nil
}

// AddRoutes add given routes into the Server.
func (s *Server) AddRoutes(rs []Route, opts ...RouteOption) {
	r := featuredRoutes{
		routes: rs,
	}
	for _, opt := range opts {
		opt(&r)
	}
	s.ngin.AddRoutes(r)
}

// AddRoute adds given route into the Server.
func (s *Server) AddRoute(r Route, opts ...RouteOption) {
	s.AddRoutes([]Route{r}, opts...)
}

// Start starts the Server.
// Graceful shutdown is enabled by default.
// Use proc.SetTimeToForceQuit to customize the graceful shutdown period.
func (s *Server) Start() {
	handleError(s.opts.start(s.ngin))
}

// Stop stops the Server.
func (s *Server) Stop() {
	logx.Close()
}

// Use adds the given middleware in the Server.
func (s *Server) Use(middleware Middleware) {
	s.ngin.use(middleware)
}

// ToMiddleware converts the given handler to a Middleware.
func ToMiddleware(handler func(next http.Handler) http.Handler) Middleware {
	return func(handle http.HandlerFunc) http.HandlerFunc {
		return handler(handle).ServeHTTP
	}
}

// WithJwt returns a func to enable jwt authentication in given route.
func WithJwt(secret string) RouteOption {
	return func(r *featuredRoutes) {
		validateSecret(secret)
		r.jwt.enabled = true
		r.jwt.secret = secret
	}
}

// WithJwtTransition returns a func to enable jwt authentication as well as jwt secret transition.
// Which means old and new jwt secrets work together for a period.
func WithJwtTransition(secret, prevSecret string) RouteOption {
	return func(r *featuredRoutes) {
		// why not validate prevSecret, because prevSecret is an already used one,
		// even it not meet our requirement, we still need to allow the transition.
		validateSecret(secret)
		r.jwt.enabled = true
		r.jwt.secret = secret
		r.jwt.prevSecret = prevSecret
	}
}

// WithMiddlewares adds given middlewares to given routes.
func WithMiddlewares(ms []Middleware, rs ...Route) []Route {
	for i := len(ms) - 1; i >= 0; i-- {
		rs = WithMiddleware(ms[i], rs...)
	}
	return rs
}

// WithMiddleware adds given middleware to given route.
func WithMiddleware(middleware Middleware, rs ...Route) []Route {
	routes := make([]Route, len(rs))

	for i := range rs {
		route := rs[i]
		routes[i] = Route{
			Method:  route.Method,
			Path:    route.Path,
			Handler: middleware(route.Handler),
		}
	}

	return routes
}

// WithNotFoundHandler returns a RunOption with not found handler set to given handler.
func WithNotFoundHandler(handler http.Handler) RunOption {
	rt := router.NewRouter()
	rt.SetNotFoundHandler(handler)
	return WithRouter(rt)
}

// WithNotAllowedHandler returns a RunOption with not allowed handler set to given handler.
func WithNotAllowedHandler(handler http.Handler) RunOption {
	rt := router.NewRouter()
	rt.SetNotAllowedHandler(handler)
	return WithRouter(rt)
}

// WithPrefix adds group as a prefix to the route paths.
func WithPrefix(group string) RouteOption {
	return func(r *featuredRoutes) {
		var routes []Route
		for _, rt := range r.routes {
			p := path.Join(group, rt.Path)
			routes = append(routes, Route{
				Method:  rt.Method,
				Path:    p,
				Handler: rt.Handler,
			})
		}
		r.routes = routes
	}
}

// WithPriority returns a RunOption with priority.
func WithPriority() RouteOption {
	return func(r *featuredRoutes) {
		r.priority = true
	}
}

// WithRouter returns a RunOption that make server run with given router.
func WithRouter(router httpx.Router) RunOption {
	return func(server *Server) {
		server.opts.start = func(ng *engine) error {
			return ng.StartWithRouter(router)
		}
	}
}

// WithSignature returns a RouteOption to enable signature verification.
func WithSignature(signature SignatureConf) RouteOption {
	return func(r *featuredRoutes) {
		r.signature.enabled = true
		r.signature.Strict = signature.Strict
		r.signature.Expiry = signature.Expiry
		r.signature.PrivateKeys = signature.PrivateKeys
	}
}

// WithTLSConfig returns a RunOption that with given tls config.
func WithTLSConfig(cfg *tls.Config) RunOption {
	return func(srv *Server) {
		srv.ngin.setTlsConfig(cfg)
	}
}

// WithUnauthorizedCallback returns a RunOption that with given unauthorized callback set.
func WithUnauthorizedCallback(callback handler.UnauthorizedCallback) RunOption {
	return func(srv *Server) {
		srv.ngin.SetUnauthorizedCallback(callback)
	}
}

// WithUnsignedCallback returns a RunOption that with given unsigned callback set.
func WithUnsignedCallback(callback handler.UnsignedCallback) RunOption {
	return func(srv *Server) {
		srv.ngin.SetUnsignedCallback(callback)
	}
}

func handleError(err error) {
	// ErrServerClosed means the server is closed manually
	if err == nil || err == http.ErrServerClosed {
		return
	}

	logx.Error(err)
	panic(err)
}

func validateSecret(secret string) {
	if len(secret) < 8 {
		panic("secret's length can't be less than 8")
	}
}
