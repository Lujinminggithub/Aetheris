package adapterhealth

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) Upsert(ctx context.Context, snapshot Snapshot) error {
	_, err := repository.pool.Exec(ctx, `INSERT INTO adapter_health_snapshots(tenant_id,device_id,adapter_id,state,capability_version,detected_format,last_scan_at,last_success_at,last_event_at,discovered,parsed,skipped,failed,lag_seconds,error_code,error_stage,component_state,component_version,protocol_version,vscode_version,last_component_heartbeat_at,pending_events,sent_events,dropped_events)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
		ON CONFLICT(tenant_id,device_id,adapter_id) DO UPDATE SET state=EXCLUDED.state,capability_version=EXCLUDED.capability_version,detected_format=EXCLUDED.detected_format,last_scan_at=EXCLUDED.last_scan_at,last_success_at=EXCLUDED.last_success_at,last_event_at=EXCLUDED.last_event_at,discovered=EXCLUDED.discovered,parsed=EXCLUDED.parsed,skipped=EXCLUDED.skipped,failed=EXCLUDED.failed,lag_seconds=EXCLUDED.lag_seconds,error_code=EXCLUDED.error_code,error_stage=EXCLUDED.error_stage,component_state=EXCLUDED.component_state,component_version=EXCLUDED.component_version,protocol_version=EXCLUDED.protocol_version,vscode_version=EXCLUDED.vscode_version,last_component_heartbeat_at=EXCLUDED.last_component_heartbeat_at,pending_events=EXCLUDED.pending_events,sent_events=EXCLUDED.sent_events,dropped_events=EXCLUDED.dropped_events,updated_at=NOW()`,
		snapshot.TenantID, snapshot.DeviceID, snapshot.AdapterID, snapshot.State, snapshot.CapabilityVersion, snapshot.DetectedFormat, snapshot.LastScanAt, snapshot.LastSuccessAt, snapshot.LastEventAt, snapshot.Discovered, snapshot.Parsed, snapshot.Skipped, snapshot.Failed, snapshot.LagSeconds, snapshot.ErrorCode, snapshot.ErrorStage, snapshot.ComponentState, snapshot.ComponentVersion, snapshot.ProtocolVersion, snapshot.VSCodeVersion, snapshot.LastComponentHeartbeatAt, snapshot.PendingEvents, snapshot.SentEvents, snapshot.DroppedEvents)
	return err
}

func (repository *Repository) List(ctx context.Context, tenantID, deviceID string) ([]Snapshot, error) {
	rows, err := repository.pool.Query(ctx, `SELECT device_id,adapter_id,state,capability_version,detected_format,last_scan_at,last_success_at,last_event_at,discovered,parsed,skipped,failed,lag_seconds,error_code,error_stage,component_state,component_version,protocol_version,vscode_version,last_component_heartbeat_at,pending_events,sent_events,dropped_events FROM adapter_health_snapshots WHERE tenant_id=$1 AND ($2='' OR device_id=$2) ORDER BY device_id,adapter_id`, tenantID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Snapshot{}
	for rows.Next() {
		var item Snapshot
		item.TenantID = tenantID
		if err := rows.Scan(&item.DeviceID, &item.AdapterID, &item.State, &item.CapabilityVersion, &item.DetectedFormat, &item.LastScanAt, &item.LastSuccessAt, &item.LastEventAt, &item.Discovered, &item.Parsed, &item.Skipped, &item.Failed, &item.LagSeconds, &item.ErrorCode, &item.ErrorStage, &item.ComponentState, &item.ComponentVersion, &item.ProtocolVersion, &item.VSCodeVersion, &item.LastComponentHeartbeatAt, &item.PendingEvents, &item.SentEvents, &item.DroppedEvents); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func SafeSnapshot(snapshot Snapshot) Snapshot {
	snapshot.TenantID = ""
	return snapshot
}
