package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/activities"
	"github.com/aetheris-dev/aetheris/server/internal/adapterhealth"
	"github.com/aetheris-dev/aetheris/server/internal/aiinteractions"
	"github.com/aetheris-dev/aetheris/server/internal/applicationpolicy"
	"github.com/aetheris-dev/aetheris/server/internal/auth"
	"github.com/aetheris-dev/aetheris/server/internal/browserpolicy"
	"github.com/aetheris-dev/aetheris/server/internal/cleaning"
	"github.com/aetheris-dev/aetheris/server/internal/config"
	"github.com/aetheris-dev/aetheris/server/internal/db"
	"github.com/aetheris-dev/aetheris/server/internal/devices"
	"github.com/aetheris-dev/aetheris/server/internal/effectiveness"
	"github.com/aetheris-dev/aetheris/server/internal/episodes"
	"github.com/aetheris-dev/aetheris/server/internal/events"
	"github.com/aetheris-dev/aetheris/server/internal/httpapi"
	"github.com/aetheris-dev/aetheris/server/internal/processknowledge"
	"github.com/aetheris-dev/aetheris/server/internal/projectattribution"
	"github.com/aetheris-dev/aetheris/server/internal/projects"
	"github.com/aetheris-dev/aetheris/server/internal/retrieval"
	"github.com/aetheris-dev/aetheris/server/internal/workroles"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.ApplyMigrations(context.Background(), pool, cfg.MigrationDir); err != nil {
		log.Fatal(err)
	}
	authService := auth.NewService(pool, cfg.AdminSessionTTL)
	if err := authService.EnsureBootstrapAdmin(context.Background(), cfg.DefaultTenantID, cfg.AdminBootstrapUsername, cfg.AdminBootstrapPassword); err != nil {
		log.Fatal(err)
	}
	location, err := time.LoadLocation(cfg.EffectivenessTimezone)
	if err != nil {
		log.Fatal(err)
	}
	effectivenessRepository := effectiveness.NewRepository(pool)
	effectivenessService := effectiveness.NewService(effectivenessRepository, pool, location)
	cleaningRepository := cleaning.NewRepository(pool)
	cleaningService := cleaning.NewService(cleaningRepository, location)
	go cleaning.NewWorker(cleaningService, cfg.DefaultTenantID, cfg.EffectivenessInterval).Run(context.Background())
	go effectiveness.NewWorker(effectivenessService, cfg.DefaultTenantID, cfg.EffectivenessInterval).Run(context.Background())
	projectAttributionRepository := projectattribution.NewRepository(pool)
	projectAttributionService := projectattribution.NewService(projectAttributionRepository)
	go projectattribution.NewWorker(projectAttributionRepository, 5*time.Second, 10000).Run(context.Background())
	adapterHealthRepository := adapterhealth.NewRepository(pool)
	processKnowledgeRepository := processknowledge.NewRepository(pool).WithEmbeddingModel(cfg.RetrievalEmbeddingModel)
	processKnowledgeService := processknowledge.NewService(processKnowledgeRepository)
	go processknowledge.NewWorker(processKnowledgeRepository, cfg.ProcessKnowledgeInterval, cfg.ProcessKnowledgeBatch).Run(context.Background())
	episodesRepository := episodes.NewRepository(pool)
	go episodes.NewWorker(episodesRepository, cfg.DefaultTenantID, 5*time.Minute).WithOllama(http.DefaultClient, cfg.OllamaURL, cfg.OllamaModel).Run(context.Background())
	var retrievalService *retrieval.QueryService
	if cfg.QdrantURL != "" && cfg.ModelGatewayURL != "" {
		retrievalRepository := retrieval.NewRepository(pool)
		embedder := retrieval.NewEmbeddingClient(cfg.ModelGatewayURL, cfg.ModelGatewayToken, cfg.RetrievalEmbeddingModel, cfg.ModelGatewayTimeout)
		vectors := retrieval.NewQdrantClient(cfg.QdrantURL, cfg.QdrantCollection, cfg.QdrantAPIKey, 30*time.Second)
		knowledgeVectors := retrieval.NewQdrantClient(cfg.QdrantURL, cfg.ProcessKnowledgeCollection, cfg.QdrantAPIKey, 30*time.Second)
		gate := retrieval.NewWorkloadGate()
		indexer := retrieval.NewIndexer(retrievalRepository, embedder, vectors, cfg.RetrievalEmbeddingModel).WithGate(gate)
		go retrieval.NewWorker(indexer, cfg.DefaultTenantID, cfg.RetrievalIndexInterval, cfg.RetrievalIndexBatch).Run(context.Background())
		retrievalService = retrieval.NewQueryService(retrievalRepository, embedder, vectors, retrieval.NewGenerator(cfg.ModelGatewayURL, cfg.ModelGatewayToken, cfg.ModelGatewayTimeout), location, cfg.RetrievalEmbeddingModel).WithGate(gate)
		knowledgeIndexer := processknowledge.NewIndexer(processKnowledgeRepository, embedder, knowledgeVectors, cfg.RetrievalEmbeddingModel)
		go processknowledge.NewIndexWorker(knowledgeIndexer, cfg.DefaultTenantID, cfg.RetrievalIndexInterval, cfg.RetrievalIndexBatch).Run(context.Background())
		retrievalService.WithKnowledge(processknowledge.NewSearcher(processKnowledgeRepository, embedder, knowledgeVectors))
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.NewRouter(httpapi.Dependencies{
		Auth:                      authService,
		Devices:                   devices.NewService(pool, cfg.DeviceEnrollmentSecret, cfg.DefaultTenantID),
		Events:                    events.NewRepository(pool),
		StaticDir:                 cfg.AdminStaticDir,
		DatabasePing:              pool.Ping,
		Pool:                      pool,
		WorkRoles:                 workroles.NewService(pool),
		ModelGatewayURL:           cfg.ModelGatewayURL,
		ModelGatewayToken:         cfg.ModelGatewayToken,
		ModelGatewayTimeout:       cfg.ModelGatewayTimeout,
		ClientDownloadFile:        cfg.ClientDownloadFile,
		Effectiveness:             effectivenessService,
		AIInteractions:            aiinteractions.NewRepository(pool),
		Activities:                activities.NewRepository(pool),
		Retrieval:                 retrievalService,
		Cleaning:                  cleaningService,
		BrowserPolicies:           browserpolicy.NewRepository(pool),
		ApplicationPolicies:       applicationpolicy.NewRepository(pool),
		ProjectIdentityKey:        cfg.ProjectIdentityKey,
		ProjectIdentityKeyVersion: cfg.ProjectIdentityKeyVersion,
		Projects:                  projects.NewRepository(pool),
		ProjectBackfills:          projectAttributionService,
		AdapterHealth:             adapterHealthRepository,
		Episodes:                  episodesRepository,
		ProcessKnowledge:          processKnowledgeService,
	})}
	log.Printf("aetheris-server listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
