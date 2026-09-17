package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aetheris-dev/aetheris/server/internal/adminops"
	"github.com/aetheris-dev/aetheris/server/internal/cleaning"
	"github.com/aetheris-dev/aetheris/server/internal/config"
	"github.com/aetheris-dev/aetheris/server/internal/db"
	"github.com/aetheris-dev/aetheris/server/internal/effectiveness"
	"github.com/aetheris-dev/aetheris/server/internal/processknowledge"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal(adminUsage())
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	switch os.Args[1] {
	case "reset-password":
		if len(os.Args) != 2 {
			log.Fatal("用法: aetheris-admin reset-password")
		}
		resetPassword(cfg)
	case "recompute-effectiveness":
		recomputeEffectiveness(cfg, os.Args[2:])
	case "recompute-cleaning":
		recomputeCleaning(cfg, os.Args[2:])
	case "process-knowledge-backfill":
		processKnowledgeBackfill(cfg, os.Args[2:])
	case "process-knowledge-export-eval":
		exportProcessKnowledgeEvaluation(cfg, os.Args[2:])
	default:
		log.Fatal(adminUsage())
	}
}

func adminUsage() string {
	return "用法: aetheris-admin reset-password | recompute-effectiveness | recompute-cleaning | process-knowledge-backfill | process-knowledge-export-eval"
}

type processKnowledgeBackfillOptions struct {
	Mode, ProjectID string
	Version         int
}

func parseProcessKnowledgeBackfillArgs(args []string) (processKnowledgeBackfillOptions, error) {
	flags := flag.NewFlagSet("process-knowledge-backfill", flag.ContinueOnError)
	mode := flags.String("mode", "apply", "dry_run 或 apply")
	project := flags.String("project", "", "逻辑项目 ID")
	version := flags.Int("version", 1, "知识版本")
	if err := flags.Parse(args); err != nil {
		return processKnowledgeBackfillOptions{}, err
	}
	if (*mode != "dry_run" && *mode != "apply") || *version < 1 {
		return processKnowledgeBackfillOptions{}, fmt.Errorf("mode/version 无效")
	}
	return processKnowledgeBackfillOptions{Mode: *mode, ProjectID: *project, Version: *version}, nil
}

func processKnowledgeBackfill(cfg config.Config, args []string) {
	options, err := parseProcessKnowledgeBackfillArgs(args)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	repository := processknowledge.NewRepository(pool).WithEmbeddingModel(cfg.RetrievalEmbeddingModel)
	service := processknowledge.NewService(repository)
	job, err := service.StartBackfill(ctx, cfg.DefaultTenantID, "aetheris-admin", options.Mode, options.ProjectID, options.Version)
	if err != nil {
		log.Fatal(err)
	}
	for {
		_, err = repository.ProcessNext(ctx, cfg.ProcessKnowledgeBatch)
		if err != nil {
			log.Fatal(err)
		}
		job, err = service.GetJob(ctx, cfg.DefaultTenantID, job.ID)
		if err != nil {
			log.Fatal(err)
		}
		if job.State == "completed" {
			log.Printf("过程知识回填完成 job=%s sessions=%d units=%d verified=%d conflicts=%d", job.ID, job.ScannedCount, job.CandidateCount, job.VerifiedCount, job.ConflictCount)
			return
		}
		if job.State == "failed" {
			log.Fatalf("过程知识回填失败 job=%s error=%s", job.ID, job.ErrorCode)
		}
	}
}

func exportProcessKnowledgeEvaluation(cfg config.Config, args []string) {
	flags := flag.NewFlagSet("process-knowledge-export-eval", flag.ExitOnError)
	output := flags.String("output", "process-knowledge-eval.jsonl", "输出 JSONL")
	_ = flags.Parse(args)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, `SELECT knowledge_id,logical_project_id,topic,problem,intent,constraints_text,conclusion,applicability,caveats FROM process_knowledge_units WHERE tenant_id=$1 AND decision_state='accepted' AND validation_state='verified' AND lifecycle_state='active' ORDER BY knowledge_id,revision DESC`, cfg.DefaultTenantID)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	file, err := os.Create(*output)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	count := 0
	for rows.Next() {
		var item map[string]string = map[string]string{}
		var id, project, topic, problem, intent, constraints, conclusion, applicability, caveats string
		if err := rows.Scan(&id, &project, &topic, &problem, &intent, &constraints, &conclusion, &applicability, &caveats); err != nil {
			log.Fatal(err)
		}
		item["knowledge_id"], item["logical_project_id"], item["topic"] = id, project, topic
		item["problem"], item["intent"], item["constraints"] = problem, intent, constraints
		item["conclusion"], item["applicability"], item["caveats"] = conclusion, applicability, caveats
		if err := encoder.Encode(item); err != nil {
			log.Fatal(err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	log.Printf("评测数据已导出 path=%s count=%d", *output, count)
}

func recomputeCleaning(cfg config.Config, args []string) {
	flags := flag.NewFlagSet("recompute-cleaning", flag.ExitOnError)
	fromRaw := flags.String("from", "", "开始日期 YYYY-MM-DD")
	toRaw := flags.String("to", "", "结束日期 YYYY-MM-DD")
	_ = flags.Parse(args)
	location, err := time.LoadLocation(cfg.EffectivenessTimezone)
	if err != nil {
		log.Fatal(err)
	}
	from, fromErr := time.ParseInLocation("2006-01-02", *fromRaw, location)
	to, toErr := time.ParseInLocation("2006-01-02", *toRaw, location)
	if fromErr != nil || toErr != nil || to.Before(from) {
		log.Fatal("from/to 必须是有效日期，且结束日期不能早于开始日期")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	service := cleaning.NewService(cleaning.NewRepository(pool), location)
	for _, item := range splitCleaningRanges(from, to) {
		facts, err := service.RunRange(ctx, cfg.DefaultTenantID, item[0], item[1], cleaning.CurrentRuleVersion)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("清洗事实回填已完成 range=%s..%s facts=%d rule=%d", item[0].Format("2006-01-02"), item[1].Format("2006-01-02"), len(facts), cleaning.CurrentRuleVersion)
	}
}

func splitCleaningRanges(from, to time.Time) [][2]time.Time {
	result := make([][2]time.Time, 0)
	for start := from; !start.After(to); {
		end := start
		result = append(result, [2]time.Time{start, end})
		start = end.AddDate(0, 0, 1)
	}
	return result
}

func resetPassword(cfg config.Config) {
	password := os.Getenv("ADMIN_RESET_PASSWORD")
	username := os.Getenv("ADMIN_RESET_USERNAME")
	if username == "" {
		username = "admin"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := adminops.ResetPassword(ctx, pool, username, password); err != nil {
		log.Fatal(err)
	}
	log.Printf("管理员 %s 的密码已重置，旧会话已撤销", username)
}

func recomputeEffectiveness(cfg config.Config, args []string) {
	flags := flag.NewFlagSet("recompute-effectiveness", flag.ExitOnError)
	fromRaw := flags.String("from", "", "开始日期 YYYY-MM-DD")
	toRaw := flags.String("to", "", "结束日期 YYYY-MM-DD")
	_ = flags.Parse(args)
	location, err := time.LoadLocation(cfg.EffectivenessTimezone)
	if err != nil {
		log.Fatal(err)
	}
	from, fromErr := time.ParseInLocation("2006-01-02", *fromRaw, location)
	to, toErr := time.ParseInLocation("2006-01-02", *toRaw, location)
	if fromErr != nil || toErr != nil || to.Before(from) {
		log.Fatal("from/to 必须是有效日期，且结束日期不能早于开始日期")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	service := effectiveness.NewService(effectiveness.NewRepository(pool), pool, location)
	for _, item := range splitEffectivenessRanges(from, to) {
		jobID, err := service.RecomputeRange(ctx, cfg.DefaultTenantID, item[0], item[1])
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("效能回填已提交 job=%s range=%s..%s", jobID, item[0].Format("2006-01-02"), item[1].Format("2006-01-02"))
		if err := waitForEffectivenessJob(ctx, pool, jobID); err != nil {
			log.Fatal(err)
		}
		log.Printf("效能回填已完成 job=%s", jobID)
	}
}

func splitEffectivenessRanges(from, to time.Time) [][2]time.Time {
	result := make([][2]time.Time, 0)
	for start := from; !start.After(to); {
		end := start.AddDate(0, 0, 30)
		if end.After(to) {
			end = to
		}
		result = append(result, [2]time.Time{start, end})
		start = end.AddDate(0, 0, 1)
	}
	return result
}

func waitForEffectivenessJob(ctx context.Context, pool *pgxpool.Pool, jobID string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var status, errorCode string
		if err := pool.QueryRow(ctx, `SELECT status,COALESCE(error_code,'') FROM effectiveness_recompute_jobs WHERE id=$1`, jobID).Scan(&status, &errorCode); err != nil {
			return err
		}
		switch status {
		case "completed":
			return nil
		case "failed":
			return fmt.Errorf("效能回填失败 job=%s error=%s", jobID, errorCode)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
