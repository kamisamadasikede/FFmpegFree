package catagent

import "FFmpegFree/internal/apperr"

// StubAdapter 是未就绪桩，仅测试用（v0.37 起生产注册真适配器 NewClaudeAdapter / NewCodexAdapter）。
// 状态恒为 missing；模型 / 强度空列表；RunTurn → CAT_NOT_READY。
type StubAdapter struct {
	kind string
}

// NewStubAdapter 创建指定 kind 的未就绪桩。
func NewStubAdapter(kind string) *StubAdapter {
	return &StubAdapter{kind: kind}
}

func (a *StubAdapter) Kind() string           { return a.kind }
func (a *StubAdapter) ExecutablePath() string { return "" }
func (a *StubAdapter) ProtocolVersion() int   { return 0 }

func (a *StubAdapter) Status() Status {
	return Status{State: StateMissing, CanDownload: false}
}

func (a *StubAdapter) Recheck() Status { return a.Status() }

func (a *StubAdapter) ListModels() ([]Model, error) { return []Model{}, nil }

func (a *StubAdapter) ListThinkLevels() ([]ThinkLevel, error) { return []ThinkLevel{}, nil }

func (a *StubAdapter) RunTurn(TurnOptions) (TurnResponse, error) {
	return TurnResponse{}, apperr.New(apperr.CatNotReady, MsgNotReady)
}
