package cat

import "context"

type groupLifetime struct {
	ctx     context.Context
	cancel  context.CancelFunc
	groupID string
}
type groupLifetimeKey struct{}

// The lifetime survives a Wails request, but not StopCatGroup. The marker also
// follows queued work and handoffs, preventing a late result from reviving work.
func (s *Service) groupContext(ctx context.Context, groupID string) context.Context {
	if life, ok := ctx.Value(groupLifetimeKey{}).(*groupLifetime); ok && life.groupID == groupID {
		return ctx
	}
	s.groupDispatchMu.Lock()
	defer s.groupDispatchMu.Unlock()
	if s.groupLifetimes == nil {
		s.groupLifetimes = map[string]*groupLifetime{}
	}
	life := s.groupLifetimes[groupID]
	if life == nil {
		root, cancel := context.WithCancel(context.Background())
		life = &groupLifetime{ctx: root, cancel: cancel, groupID: groupID}
		s.groupLifetimes[groupID] = life
	}
	return context.WithValue(ctx, groupLifetimeKey{}, life)
}

func groupContextStopped(ctx context.Context) bool {
	if ctx.Err() != nil {
		return true
	}
	life, ok := ctx.Value(groupLifetimeKey{}).(*groupLifetime)
	return ok && life.ctx.Err() != nil
}

func groupTurnContext(ctx context.Context) (context.Context, context.CancelFunc) {
	turn, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := func() bool { return false }
	if life, ok := ctx.Value(groupLifetimeKey{}).(*groupLifetime); ok {
		stop = context.AfterFunc(life.ctx, cancel)
		if life.ctx.Err() != nil {
			cancel()
		}
	}
	return turn, func() { stop(); cancel() }
}

func (s *Service) groupRequestContext(ctx context.Context, groupID string) (context.Context, context.CancelFunc) {
	ctx = s.groupContext(ctx, groupID)
	route, cancel := context.WithCancel(ctx)
	life := ctx.Value(groupLifetimeKey{}).(*groupLifetime)
	stop := context.AfterFunc(life.ctx, cancel)
	if life.ctx.Err() != nil {
		cancel()
	}
	return route, func() { stop(); cancel() }
}
