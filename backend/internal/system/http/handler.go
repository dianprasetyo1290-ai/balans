package http

import (
	"context"

	"balans/internal/system/service"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/system.yaml ../../../api/openapi.bundle.yaml

type systemReader interface {
	Info(ctx context.Context) service.Info
	Readiness(ctx context.Context) service.Readiness
}

// SystemHandler mengimplementasikan StrictServerInterface hasil generate.
type SystemHandler struct {
	system systemReader
}

var _ StrictServerInterface = (*SystemHandler)(nil)

// NewSystemHandler membuat handler.
func NewSystemHandler(system systemReader) *SystemHandler {
	return &SystemHandler{system: system}
}

// GetHealth menandakan proses hidup.
func (h *SystemHandler) GetHealth(context.Context, GetHealthRequestObject) (GetHealthResponseObject, error) {
	return GetHealth200JSONResponse{Status: HealthStatusStatusOk}, nil
}

// GetReadiness menandakan proses siap menerima trafik.
func (h *SystemHandler) GetReadiness(ctx context.Context, _ GetReadinessRequestObject) (GetReadinessResponseObject, error) {
	r := h.system.Readiness(ctx)
	body := ReadinessStatus{Status: NotReady, Checks: make([]ReadinessCheck, 0, len(r.Checks))}
	for _, c := range r.Checks {
		check := ReadinessCheck{Name: c.Name, Status: ReadinessCheckStatusFailed}
		if c.OK {
			check.Status = ReadinessCheckStatusOk
		}
		if c.Detail != "" {
			check.Detail = &c.Detail
		}
		body.Checks = append(body.Checks, check)
	}
	if !r.Ready {
		return GetReadiness503JSONResponse(body), nil
	}
	body.Status = Ready
	return GetReadiness200JSONResponse(body), nil
}

// GetSystemInfo mengembalikan informasi sistem.
func (h *SystemHandler) GetSystemInfo(ctx context.Context, _ GetSystemInfoRequestObject) (GetSystemInfoResponseObject, error) {
	info := h.system.Info(ctx)
	dbStatus := DatabaseInfoStatusFailed
	if info.Database.OK {
		dbStatus = DatabaseInfoStatusOk
	}
	modules := info.Build.EnabledModules
	if modules == nil {
		modules = []string{}
	}
	return GetSystemInfo200JSONResponse{
		Version:        info.Build.Version,
		Commit:         info.Build.Commit,
		Environment:    SystemInfoEnvironment(info.Build.Environment),
		EnabledModules: modules,
		MockActive:     info.Build.MockActive,
		ServerTime:     info.ServerTime,
		Database: DatabaseInfo{
			Status:              dbStatus,
			SchemaVersion:       info.Database.SchemaVersion,
			LatestSchemaVersion: info.Database.LatestSchemaVersion,
		},
	}, nil
}
