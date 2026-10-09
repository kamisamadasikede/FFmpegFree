package doc

import (
	"context"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/doccomp"
)

// DocComponentStatus 是契约 6.12.13 / 6.12.28 的组件状态（v0.26.1 加 installBytes；v0.27 加 componentState、engines，
// state 改为“文档转换整体是否可用”——本版只有文档组件一个引擎，所以 state 恒等于 componentState）。
// 与 doccomp.Status 字段一一对应，单独定义是为了生成的前端类型名叫 doc.DocComponentStatus。
type DocComponentStatus struct {
	State          string           `json:"state"`
	ComponentState string           `json:"componentState"`
	Engines        []DocEngineInfo  `json:"engines"`
	Version        string           `json:"version"`
	Source         string           `json:"source"`
	CanDownload    bool             `json:"canDownload"`
	DownloadBytes  int64            `json:"downloadBytes"`
	InstallBytes   int64            `json:"installBytes"`
	Phase          string           `json:"phase,omitempty"`
	ReceivedBytes  int64            `json:"receivedBytes,omitempty"`
	Error          *apperr.AppError `json:"error,omitempty"`
}

// DocEngineInfo 见契约 6.12.28。
type DocEngineInfo struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Source    string   `json:"source,omitempty"`
	Installed bool     `json:"installed"`
	Families  []string `json:"families"`
	Available bool     `json:"available"`
}

func fromStatus(s doccomp.Status) DocComponentStatus {
	engines := make([]DocEngineInfo, 0, len(s.Engines))
	for _, e := range s.Engines {
		engines = append(engines, DocEngineInfo(e))
	}
	return DocComponentStatus{State: s.State, ComponentState: s.ComponentState, Engines: engines, Version: s.Version, Source: s.Source, CanDownload: s.CanDownload,
		DownloadBytes: s.DownloadBytes, InstallBytes: s.InstallBytes, Phase: s.Phase, ReceivedBytes: s.ReceivedBytes, Error: s.Error}
}

// ComponentInstaller 是安装相关能力（*doccomp.Manager 实现）。
type ComponentInstaller interface {
	Install(mirror string) (doccomp.Status, error)
	Cancel()
	Recheck() doccomp.Status
}

func (s *Service) installer() (ComponentInstaller, error) {
	if ci, ok := s.cfg.Component.(ComponentInstaller); ok && s.cfg.Component != nil {
		return ci, nil
	}
	return nil, s.internalNotReady("文档组件")
}

// GetDocComponentStatus 返回组件状态（不含路径）。
func (s *Service) GetDocComponentStatus(ctx context.Context) (DocComponentStatus, error) {
	if s.cfg.Component == nil {
		return DocComponentStatus{}, s.internalNotReady("文档组件")
	}
	return fromStatus(s.cfg.Component.Status()), nil
}

// InstallDocComponent 开始或继续下载 + 准备；幂等；Linux UNSUPPORTED_PLATFORM。
func (s *Service) InstallDocComponent(ctx context.Context, mirror string) (DocComponentStatus, error) {
	ci, err := s.installer()
	if err != nil {
		return DocComponentStatus{}, err
	}
	st, err := ci.Install(mirror)
	if err != nil {
		return DocComponentStatus{}, err
	}
	return fromStatus(st), nil
}

// CancelDocComponentInstall 取消下载 / 准备；没有在进行时无操作。
func (s *Service) CancelDocComponentInstall(ctx context.Context) error {
	ci, err := s.installer()
	if err != nil {
		return err
	}
	ci.Cancel()
	return nil
}

// RecheckDocComponent 重新检测；下载 / 准备中返回当前状态，不打断。
func (s *Service) RecheckDocComponent(ctx context.Context) (DocComponentStatus, error) {
	ci, err := s.installer()
	if err != nil {
		return DocComponentStatus{}, err
	}
	return fromStatus(ci.Recheck()), nil
}
