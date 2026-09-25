package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/amneziawg"
	"github.com/mhsanaei/3x-ui/v3/internal/amneziawgnet"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/l2tp"
	"github.com/mhsanaei/3x-ui/v3/internal/mtproto"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type LocalDeps struct {
	APIPort        func() int
	SetNeedRestart func()
}

type Local struct {
	deps LocalDeps
	mu   sync.Mutex
}

func NewLocal(deps LocalDeps) *Local {
	return &Local{deps: deps}
}

func (l *Local) Name() string { return "local" }

func (l *Local) withAPI(fn func(api *xray.XrayAPI) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	port := l.deps.APIPort()
	if port <= 0 {
		return errors.New("local xray is not running")
	}
	var api xray.XrayAPI
	if err := api.Init(port); err != nil {
		return err
	}
	defer api.Close()
	return fn(&api)
}

func amneziaWGOptions(inst amneziawg.Instance) amneziawgnet.DeviceOptions {
	return amneziawgnet.DeviceOptions{
		HeaderProtectionKey:    inst.Obfuscation.HeaderProtectionKey,
		ContentPaddingAddition: inst.Obfuscation.ContentPaddingAddition,
		RekeyAfterTime:         inst.Obfuscation.RekeyAfterTime,
		RekeyTimeout:           inst.Obfuscation.RekeyTimeout,
		RejectAfterTime:        inst.Obfuscation.RejectAfterTime,
		KeepaliveTimeout:       inst.Obfuscation.KeepaliveTimeout,
		MaxHandshakeAttempts:   inst.Obfuscation.MaxHandshakeAttempts,
		RandomTrailers:         inst.Obfuscation.RandomTrailers,
		DisableCookies:         inst.Obfuscation.DisableCookies,
	}
}

func (l *Local) AddInbound(_ context.Context, ib *model.Inbound) error {
	if ib.Protocol == model.OpenVPN {
		// OpenVPN is reconciled by the standalone daemon job. Do not send its
		// panel model to the Xray API; the next job tick will start it from the
		// committed database state.
		return nil
	}
	if ib.Protocol == model.MTProto {
		inst, ok := mtproto.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		return mtproto.GetManager().Ensure(inst)
	}
	if ib.Protocol == model.AmneziaWG {
		inst, ok := amneziawg.InstanceFromInbound(ib)
		if !ok {
			return nil
		}
		if err := amneziawgnet.GetManager().Ensure(amneziawgnet.Desired{Instance: inst, Options: amneziaWGOptions(inst)}); err != nil {
			return err
		}
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return nil
	}
	if ib.Protocol == model.L2TP {
		inst, ok := l2tp.InstanceFromInbound(ib, nil)
		if !ok {
			return errors.New("invalid l2tp inbound settings")
		}
		return l2tp.GetManager().Ensure(inst)
	}
	body, err := json.MarshalIndent(ib.GenXrayInboundConfig(), "", "  ")
	if err != nil {
		return err
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.AddInbound(body)
	})
}

func (l *Local) DelInbound(_ context.Context, ib *model.Inbound) error {
	if ib.Protocol == model.OpenVPN {
		openvpn.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.MTProto {
		mtproto.GetManager().Remove(ib.Id)
		return nil
	}
	if ib.Protocol == model.AmneziaWG {
		amneziawgnet.GetManager().Remove(ib.Id)
		if l.deps.SetNeedRestart != nil {
			l.deps.SetNeedRestart()
		}
		return nil
	}
	if ib.Protocol == model.L2TP {
		l2tp.GetManager().Remove(ib.Id)
		return nil
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.DelInbound(ib.Tag)
	})
}

func (l *Local) UpdateInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if oldIb.Protocol == model.OpenVPN && newIb.Protocol == model.OpenVPN {
		// The OpenVPN job owns daemon restarts and reads the committed DB
		// snapshot, so an edit or enable toggle must not stop a live daemon.
		return nil
	}
	if oldIb.Protocol == model.OpenVPN || newIb.Protocol == model.OpenVPN {
		// A protocol conversion still has to remove the old runtime, but the
		// OpenVPN side must never be sent to the Xray API.
		if err := l.DelInbound(ctx, oldIb); err != nil {
			return err
		}
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol == model.AmneziaWG || newIb.Protocol == model.AmneziaWG {
		return l.updateAmneziaWGInbound(ctx, oldIb, newIb)
	}
	_ = l.DelInbound(ctx, oldIb)
	if !newIb.Enable {
		return nil
	}
	return l.AddInbound(ctx, newIb)
}

func (l *Local) updateAmneziaWGInbound(ctx context.Context, oldIb, newIb *model.Inbound) error {
	if l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	if oldIb.Protocol == model.AmneziaWG && newIb.Protocol != model.AmneziaWG {
		amneziawgnet.GetManager().Remove(oldIb.Id)
		if !newIb.Enable {
			return nil
		}
		return l.AddInbound(ctx, newIb)
	}
	if oldIb.Protocol != model.AmneziaWG {
		_ = l.DelInbound(ctx, oldIb)
	}
	if !newIb.Enable {
		amneziawgnet.GetManager().Remove(newIb.Id)
		return nil
	}
	inst, ok := amneziawg.InstanceFromInbound(newIb)
	if !ok {
		amneziawgnet.GetManager().Remove(newIb.Id)
		return nil
	}
	return amneziawgnet.GetManager().Ensure(amneziawgnet.Desired{Instance: inst, Options: amneziaWGOptions(inst)})
}

func (l *Local) AddUser(_ context.Context, ib *model.Inbound, userMap map[string]any) error {
	if ib.Protocol == model.OpenVPN || ib.Protocol == model.MTProto || ib.Protocol == model.AmneziaWG || ib.Protocol == model.L2TP {
		return nil
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.AddUser(string(ib.Protocol), ib.Tag, userMap)
	})
}

func (l *Local) RemoveUser(_ context.Context, ib *model.Inbound, email string) error {
	if ib.Protocol == model.OpenVPN || ib.Protocol == model.MTProto || ib.Protocol == model.AmneziaWG || ib.Protocol == model.L2TP {
		return nil
	}
	return l.withAPI(func(api *xray.XrayAPI) error {
		return api.RemoveUser(ib.Tag, email)
	})
}

func (l *Local) AddClient(ctx context.Context, ib *model.Inbound, client model.Client) error {
	if !client.Enable {
		return nil
	}
	user := map[string]any{
		"email":    client.Email,
		"id":       client.ID,
		"security": client.Security,
		"flow":     client.Flow,
		"auth":     client.Auth,
		"password": client.Password,
	}
	return l.AddUser(ctx, ib, user)
}

func (l *Local) DeleteUser(ctx context.Context, ib *model.Inbound, email string) error {
	if email == "" {
		return nil
	}
	if err := l.RemoveUser(ctx, ib, email); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return err
	}
	return nil
}

func (l *Local) UpdateUser(ctx context.Context, ib *model.Inbound, oldEmail string, payload model.Client) error {
	if oldEmail != "" {
		if err := l.RemoveUser(ctx, ib, oldEmail); err != nil && !strings.Contains(err.Error(), "not found") {
			return err
		}
	}
	if !payload.Enable {
		return nil
	}
	user := map[string]any{
		"email":    payload.Email,
		"id":       payload.ID,
		"security": payload.Security,
		"flow":     payload.Flow,
		"auth":     payload.Auth,
		"password": payload.Password,
	}
	return l.AddUser(ctx, ib, user)
}

func (l *Local) RestartXray(_ context.Context) error {
	if l.deps.SetNeedRestart != nil {
		l.deps.SetNeedRestart()
	}
	return nil
}

func (l *Local) ResetClientTraffic(_ context.Context, _ *model.Inbound, _ string) error {
	return nil
}

func (l *Local) ResetAllTraffics(_ context.Context) error {
	return nil
}

func (l *Local) ResetInboundTraffic(_ context.Context, _ *model.Inbound) error {
	return nil
}
