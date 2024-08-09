package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-puzzles/plog"
	"github.com/go-puzzles/prouter"
	"github.com/gorilla/mux"
)

type routers []prouter.Route

func (r routers) Routes() []prouter.Route {
	return r
}

var (
	myRouters = routers{prouter.NewRoute(http.MethodGet, "/test", helloHandler)}
)

func helloHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) (prouter.Response, error) {
	panic("test err")
	// return prouter.SuccessResponse("hello world"), nil
}

func testMiddleware(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string) (prouter.Response, error) {
	fmt.Println(111)
	return nil, nil
}

func main() {
	prouter.SetMode(prouter.DebugMode)
	router := prouter.NewProuter()

	router.HandleRoute(http.MethodGet, "/hello/{name}", helloHandler, func(route *mux.Route) *mux.Route {
		return route.Headers("Content-Type", "application/json", "X-Requested-With", "XMLHttpRequest")
	})
	router.HandlerRouter(myRouters)
	router.Static("/static", "./content")

	group := router.Group("/group1")
	group.UseMiddleware(prouter.HandleFunc(testMiddleware))
	group.HandleRoute(http.MethodGet, "/hello2/{name}", helloHandler)

	srv := http.Server{
		Addr:    ":8080",
		Handler: router,
	}
	plog.PanicError(srv.ListenAndServe())
}
