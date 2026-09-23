package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ronavpoonekar/forge/api"
	"github.com/ronavpoonekar/forge/proto/forgepb"
	"github.com/ronavpoonekar/forge/scheduler"
	"github.com/ronavpoonekar/forge/store"
	"google.golang.org/grpc"
)

func main() {
	httpAddr := flag.String("http", "127.0.0.1:8080", "HTTP listen address")
	grpcAddr := flag.String("grpc", "127.0.0.1:50051", "gRPC listen address")
	webDir := flag.String("web-dir", "web/dist", "compiled dashboard directory")
	demo := flag.Bool("demo-controls", false, "enable dashboard worker failure injection")
	flag.Parse()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/forge?sslmode=disable"
	}
	st, err := store.New(dsn)
	if err != nil {
		log.Fatalf("Connect to database: %v", err)
	}
	defer st.Close()
	sched := scheduler.New(st)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go sched.StartLeaseChecker(ctx, 10*time.Second, 15*time.Second)
	grpcServer := grpc.NewServer()
	forgepb.RegisterForgeServiceServer(grpcServer, scheduler.NewGRPCServer(sched))
	listener, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("gRPC: %v", err)
			cancel()
		}
	}()
	server := api.New(sched, st, *demo)
	go server.Run(ctx)
	httpServer := &http.Server{Addr: *httpAddr, Handler: server.Handler(*webDir), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("Dashboard: http://%s · gRPC: %s · demo controls: %t", *httpAddr, *grpcAddr, *demo)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP: %v", err)
			cancel()
		}
	}()
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	httpServer.Shutdown(shutdown)
	grpcServer.Stop()
}
