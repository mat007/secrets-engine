// Copyright 2026 Docker, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secretservice

import (
	"context"
	"testing"
	"time"

	dbus "github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

const (
	propertiesInterface = "org.freedesktop.DBus.Properties"
	loginCollectionPath = dbus.ObjectPath("/org/freedesktop/secrets/collection/login")
)

type blockedCall struct {
	entered chan struct{}
	release chan struct{}
}

func newBlockedCall(t *testing.T) *blockedCall {
	t.Helper()
	blocked := &blockedCall{entered: make(chan struct{}, 1), release: make(chan struct{})}
	t.Cleanup(func() { close(blocked.release) })
	return blocked
}

func (b *blockedCall) block() {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
}

func (b *blockedCall) Get(string, string) (dbus.Variant, *dbus.Error) {
	b.block()
	return dbus.MakeVariant([]dbus.ObjectPath{loginCollectionPath}), nil
}

func (b *blockedCall) ReadAlias(string) (dbus.ObjectPath, *dbus.Error) {
	b.block()
	return loginCollectionPath, nil
}

func TestCollectionsCallerDeadline(t *testing.T) {
	svc, backend := newDelayedSessionService(t)
	svc.SetCallTimeout(5 * time.Second)
	blocked := newBlockedCall(t)
	require.NoError(t, backend.conn.Export(blocked, SecretServiceObjectPath, propertiesInterface))

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := svc.Collections(ctx)
		result <- err
	}()

	receiveSessionValue(t, blocked.entered)
	require.ErrorIs(t, receiveSessionValue(t, result), context.DeadlineExceeded)
	require.True(t, svc.conn.Connected(), "a timed-out property read must not disconnect the service")
}

func TestCollectionsCapsCallerWithoutDeadline(t *testing.T) {
	svc, backend := newDelayedSessionService(t)
	blocked := newBlockedCall(t)
	require.NoError(t, backend.conn.Export(blocked, SecretServiceObjectPath, propertiesInterface))

	result := make(chan error, 1)
	go func() {
		_, err := svc.Collections(context.Background())
		result <- err
	}()

	receiveSessionValue(t, blocked.entered)
	require.ErrorIs(t, receiveSessionValue(t, result), context.DeadlineExceeded)
}

func TestIsLockedCallerDeadline(t *testing.T) {
	svc, backend := newDelayedSessionService(t)
	svc.SetCallTimeout(5 * time.Second)
	blocked := newBlockedCall(t)
	require.NoError(t, backend.conn.Export(blocked, loginCollectionPath, propertiesInterface))

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := svc.IsLocked(ctx, loginCollectionPath)
		result <- err
	}()

	receiveSessionValue(t, blocked.entered)
	require.ErrorIs(t, receiveSessionValue(t, result), context.DeadlineExceeded)
}

func TestReadAliasCallerDeadline(t *testing.T) {
	svc, backend := newDelayedSessionService(t)
	svc.SetCallTimeout(5 * time.Second)
	blocked := newBlockedCall(t)
	require.NoError(t, backend.conn.Export(blocked, SecretServiceObjectPath, secretServiceInterfaceName))

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := svc.ReadAlias(ctx, "default")
		result <- err
	}()

	receiveSessionValue(t, blocked.entered)
	require.ErrorIs(t, receiveSessionValue(t, result), context.DeadlineExceeded)
	require.True(t, svc.conn.Connected())
}
