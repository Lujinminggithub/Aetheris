package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr                   string
	DatabaseURL                string
	AdminSessionTTL            time.Duration
	ModelGatewayURL            string
	ModelGatewayToken          string
	ModelGatewayTimeout        time.Duration
	OllamaURL                  string
	OllamaModel                string
	AdminStaticDir             string
	MigrationDir               string
	DeviceEnrollmentSecret     string
	DefaultTenantID            string
	AdminBootstrapUsername     string
	AdminBootstrapPassword     string
	ClientDownloadFile         string
	EffectivenessTimezone      string
	EffectivenessInterval      time.Duration
	QdrantURL                  string
	QdrantAPIKey               string
	QdrantCollection           string
	RetrievalEmbeddingModel    string
	RetrievalIndexInterval     time.Duration
	RetrievalIndexBatch        int
	ProcessKnowledgeCollection string
	ProcessKnowledgeInterval   time.Duration
	ProcessKnowledgeBatch      int
	PublicKnowledgeCollection  string
	PublicKnowledgeInterval    time.Duration
	PublicKnowledgeBatch       int
	ProjectIdentityKey         []byte
	ProjectIdentityKeyVersion  int
}

func Load() (Config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	ttl := 12 * time.Hour
	if raw := os.Getenv("ADMIN_SESSION_TTL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 300 {
			return Config{}, fmt.Errorf("ADMIN_SESSION_TTL must be at least 300 seconds")
		}
		ttl = time.Duration(seconds) * time.Second
	}
	effectivenessInterval := 5 * time.Minute
	if raw := os.Getenv("EFFECTIVENESS_RECOMPUTE_INTERVAL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 60 {
			return Config{}, fmt.Errorf("EFFECTIVENESS_RECOMPUTE_INTERVAL must be at least 60 seconds")
		}
		effectivenessInterval = time.Duration(seconds) * time.Second
	}
	modelGatewayTimeout := 600 * time.Second
	if raw := os.Getenv("MODEL_GATEWAY_TIMEOUT"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 5 || seconds > 600 {
			return Config{}, fmt.Errorf("MODEL_GATEWAY_TIMEOUT must be between 5 and 600 seconds")
		}
		modelGatewayTimeout = time.Duration(seconds) * time.Second
	}
	retrievalInterval := 10 * time.Second
	if raw := os.Getenv("RETRIEVAL_INDEX_INTERVAL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 2 {
			return Config{}, fmt.Errorf("RETRIEVAL_INDEX_INTERVAL must be at least 2 seconds")
		}
		retrievalInterval = time.Duration(seconds) * time.Second
	}
	retrievalBatch := 8
	if raw := os.Getenv("RETRIEVAL_INDEX_BATCH"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 64 {
			return Config{}, fmt.Errorf("RETRIEVAL_INDEX_BATCH must be between 1 and 64")
		}
		retrievalBatch = value
	}
	processKnowledgeInterval := 60 * time.Second
	if raw := os.Getenv("PROCESS_KNOWLEDGE_INTERVAL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 2 {
			return Config{}, fmt.Errorf("PROCESS_KNOWLEDGE_INTERVAL must be at least 2 seconds")
		}
		processKnowledgeInterval = time.Duration(seconds) * time.Second
	}
	processKnowledgeBatch := 100
	if raw := os.Getenv("PROCESS_KNOWLEDGE_BATCH"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 1000 {
			return Config{}, fmt.Errorf("PROCESS_KNOWLEDGE_BATCH must be between 1 and 1000")
		}
		processKnowledgeBatch = value
	}
	publicKnowledgeInterval := 5 * time.Minute
	if raw := os.Getenv("PUBLIC_KNOWLEDGE_INTERVAL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 30 {
			return Config{}, fmt.Errorf("PUBLIC_KNOWLEDGE_INTERVAL must be at least 30 seconds")
		}
		publicKnowledgeInterval = time.Duration(seconds) * time.Second
	}
	publicKnowledgeBatch := 25
	if raw := os.Getenv("PUBLIC_KNOWLEDGE_BATCH"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return Config{}, fmt.Errorf("PUBLIC_KNOWLEDGE_BATCH must be between 1 and 100")
		}
		publicKnowledgeBatch = value
	}
	projectIdentityKeyRaw := os.Getenv("AETHERIS_PROJECT_IDENTITY_KEY")
	projectIdentityKey, err := base64.StdEncoding.DecodeString(projectIdentityKeyRaw)
	if err != nil || len(projectIdentityKey) < 32 {
		return Config{}, fmt.Errorf("AETHERIS_PROJECT_IDENTITY_KEY 必须是解码后不少于 32 字节的 Base64 密钥")
	}
	projectIdentityKeyVersion := 1
	if raw := os.Getenv("AETHERIS_PROJECT_IDENTITY_KEY_VERSION"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return Config{}, fmt.Errorf("AETHERIS_PROJECT_IDENTITY_KEY_VERSION 必须为正整数")
		}
		projectIdentityKeyVersion = value
	}
	return Config{
		HTTPAddr:                   envOr("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:                databaseURL,
		AdminSessionTTL:            ttl,
		ModelGatewayURL:            os.Getenv("MODEL_GATEWAY_URL"),
		ModelGatewayToken:          os.Getenv("MODEL_GATEWAY_TOKEN"),
		ModelGatewayTimeout:        modelGatewayTimeout,
		OllamaURL:                  envOr("OLLAMA_URL", ""),
		OllamaModel:                envOr("OLLAMA_MODEL", "qwen3:1.7b"),
		AdminStaticDir:             envOr("ADMIN_STATIC_DIR", "../admin-web/dist"),
		MigrationDir:               envOr("MIGRATION_DIR", "migrations"),
		DeviceEnrollmentSecret:     os.Getenv("DEVICE_ENROLLMENT_SECRET"),
		DefaultTenantID:            envOr("DEFAULT_TENANT_ID", "tenant-local"),
		AdminBootstrapUsername:     envOr("ADMIN_BOOTSTRAP_USERNAME", "admin"),
		AdminBootstrapPassword:     os.Getenv("ADMIN_BOOTSTRAP_PASSWORD"),
		ClientDownloadFile:         os.Getenv("CLIENT_DOWNLOAD_FILE"),
		EffectivenessTimezone:      envOr("EFFECTIVENESS_TIMEZONE", "Asia/Shanghai"),
		EffectivenessInterval:      effectivenessInterval,
		QdrantURL:                  os.Getenv("QDRANT_URL"),
		QdrantAPIKey:               os.Getenv("QDRANT_API_KEY"),
		QdrantCollection:           envOr("QDRANT_COLLECTION", "aetheris_activities_v1"),
		RetrievalEmbeddingModel:    envOr("RETRIEVAL_EMBEDDING_MODEL", "embeddinggemma"),
		RetrievalIndexInterval:     retrievalInterval,
		RetrievalIndexBatch:        retrievalBatch,
		ProcessKnowledgeCollection: envOr("PROCESS_KNOWLEDGE_COLLECTION", "aetheris_process_knowledge_v1"),
		ProcessKnowledgeInterval:   processKnowledgeInterval,
		ProcessKnowledgeBatch:      processKnowledgeBatch,
		PublicKnowledgeCollection:  envOr("PUBLIC_KNOWLEDGE_COLLECTION", "aetheris_public_knowledge_v1"),
		PublicKnowledgeInterval:    publicKnowledgeInterval,
		PublicKnowledgeBatch:       publicKnowledgeBatch,
		ProjectIdentityKey:         projectIdentityKey,
		ProjectIdentityKeyVersion:  projectIdentityKeyVersion,
	}, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
