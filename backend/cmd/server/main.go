package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daudalobogachiev925/realtime-chat/backend/internal/auth"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/chat"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/message"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/presence"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/pubsub"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/room"
	"github.com/daudalobogachiev925/realtime-chat/backend/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	addr := env("HTTP_ADDR", ":8080")
	dbURL := env("DATABASE_URL", "postgres://chat:chat@localhost:5432/chat?sslmode=disable")
	redisAddr := env("REDIS_ADDR", "localhost:6379")
	jwtSecret := env("JWT_SECRET", "dev-secret-change-me")

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	st := store.New(pool)
	authSvc := auth.New(jwtSecret)
	bus := pubsub.New(rdb)
	pres := presence.New(rdb)
	hub := chat.NewHub()
	svc := chat.NewService(hub, st, bus, pres)

	// подписка ноды на Redis Pub/Sub + heartbeat presence
	svc.RunSubscriber(ctx)
	go svc.RunPresenceHeartbeat(ctx)

	authH := auth.NewHandlers(st, authSvc)
	roomH := room.NewHandlers(st, authSvc)
	msgH := message.NewHandlers(st, authSvc)
	wsH := chat.NewWSHandler(hub, svc, authSvc)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/api", func(r chi.Router) {
		r.Post("/register", authH.Register)
		r.Post("/login", authH.Login)
		r.Get("/me", authH.Me)

		r.Get("/rooms", roomH.List)
		r.Post("/rooms", roomH.Create)

		r.Get("/rooms/{roomID}/messages", msgH.List)
		r.Get("/messages/{messageID}/replies", msgH.Replies)
		r.Get("/messages/{messageID}/reactions", msgH.Reactions)
	})

	r.Get("/ws/rooms/{roomID}", wsH.Serve)

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancelShut := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShut()
	_ = srv.Shutdown(shutdownCtx)
}
