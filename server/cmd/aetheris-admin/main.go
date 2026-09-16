package main

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: aetheris-admin reset-password | recompute-effectiveness --from YYYY-MM-DD --to YYYY-MM-DD | recompute-cleaning --from YYYY-MM-DD --to YYYY-MM-DD")
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
	default:
		log.Fatal("用法: aetheris-admin reset-password | recompute-effectiveness --from YYYY-MM-DD --to YYYY-MM-DD | recompute-cleaning --from YYYY-MM-DD --to YYYY-MM-DD")
	}
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
