/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	tlspkg "github.com/opendatahub-io/odh-platform-utilities/framework/tls"
	configv1 "github.com/openshift/api/config/v1"
	commonTLS "github.com/openshift/controller-runtime-common/pkg/tls"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func tlsTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()
	utilruntime.Must(configv1.AddToScheme(s))

	return s
}

func apiServerClient(
	t *testing.T,
	profile *configv1.TLSSecurityProfile,
	adherence configv1.TLSAdherencePolicy,
) client.Client {
	t.Helper()

	obj := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: commonTLS.APIServerName},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: profile,
			TLSAdherence:       adherence,
		},
	}

	return fake.NewClientBuilder().WithScheme(tlsTestScheme(t)).WithObjects(obj).Build()
}

// errorReader simulates APIServer Get failures that cannot be expressed with
// an object-backed fake client.
type errorReader struct {
	err error
}

func (r errorReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return r.err
}

func (r errorReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return r.err
}

func forbiddenError() error {
	return apierrors.NewForbidden(
		schema.GroupResource{Group: "config.openshift.io", Resource: "apiservers"},
		commonTLS.APIServerName,
		errors.New("rbac denied"),
	)
}

const http1Proto = "http/1.1"

func TestManagerSchemeRegistersAPIServer(t *testing.T) {
	if !scheme.Recognizes(configv1.GroupVersion.WithKind("APIServer")) {
		t.Fatal("manager scheme does not register configv1.APIServer")
	}
}

func TestManagerTLSOptions(t *testing.T) {
	tests := []struct {
		name           string
		profile        *configv1.TLSSecurityProfile
		adherence      configv1.TLSAdherencePolicy
		reader         client.Reader
		enableHTTP2    bool
		wantErr        bool
		wantMin        uint16
		wantCiphers    bool
		wantOldCiphers bool
		wantProtos     []string
		wantWatchable  bool
		wantFallback   bool
	}{
		{
			name:          "strict intermediate honors profile with http/1.1 only",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
			adherence:     configv1.TLSAdherencePolicyStrictAllComponents,
			enableHTTP2:   false,
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name:          "strict intermediate honors profile with http/2",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType},
			adherence:     configv1.TLSAdherencePolicyStrictAllComponents,
			enableHTTP2:   true,
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{"h2", http1Proto},
			wantWatchable: true,
		},
		{
			name:          "strict modern honors TLS 1.3 without cipher suites",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			adherence:     configv1.TLSAdherencePolicyStrictAllComponents,
			wantMin:       tls.VersionTLS13,
			wantCiphers:   false,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name:           "strict old profile is honored",
			profile:        &configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType},
			adherence:      configv1.TLSAdherencePolicyStrictAllComponents,
			wantMin:        tls.VersionTLS10,
			wantOldCiphers: true,
			wantProtos:     []string{http1Proto},
			wantWatchable:  true,
		},
		{
			name: "strict custom profile maps ciphers",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
			adherence:     configv1.TLSAdherencePolicyStrictAllComponents,
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name: "strict custom profile with all unusable ciphers fails closed",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"DHE-RSA-AES128-GCM-SHA256", "TLS_AES_128_GCM_SHA256"},
					},
				},
			},
			adherence: configv1.TLSAdherencePolicyStrictAllComponents,
			wantErr:   true,
		},
		{
			name: "strict custom profile honors TLS 1.0 minimum",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS10,
						Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
					},
				},
			},
			adherence:     configv1.TLSAdherencePolicyStrictAllComponents,
			wantMin:       tls.VersionTLS10,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name: "strict custom profile without a spec fails closed",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
			},
			adherence: configv1.TLSAdherencePolicyStrictAllComponents,
			wantErr:   true,
		},
		{
			name: "strict unknown profile type fails closed",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileType("FutureProfile"),
			},
			adherence: configv1.TLSAdherencePolicyStrictAllComponents,
			wantErr:   true,
		},
		{
			name:          "non-strict unknown profile uses intermediate defaults",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileType("FutureProfile")},
			adherence:     configv1.TLSAdherencePolicyNoOpinion,
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name:          "nil profile with strict adherence uses intermediate",
			profile:       nil,
			adherence:     configv1.TLSAdherencePolicyStrictAllComponents,
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name:          "no opinion does not honor cluster profile",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			adherence:     configv1.TLSAdherencePolicyNoOpinion,
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name:          "legacy adhering components only does not honor cluster profile",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			adherence:     configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name:          "unrecognized adherence value honors cluster profile",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			adherence:     configv1.TLSAdherencePolicy("MisconfiguredFutureValue"),
			wantMin:       tls.VersionTLS13,
			wantCiphers:   false,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			name:          "unset adherence keeps hardened defaults",
			profile:       &configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType},
			adherence:     "",
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
		},
		{
			// With framework v0.4.0, NotFound (the API is served but the
			// APIServer object is missing) returns Watchable=true so the
			// watcher registers and catches the object's creation.
			name:          "missing apiserver falls back to intermediate with watcher",
			reader:        fake.NewClientBuilder().WithScheme(tlsTestScheme(t)).Build(),
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
			wantFallback:  true,
		},
		{
			name:          "forbidden apiserver get fails closed",
			reader:        errorReader{err: forbiddenError()},
			wantErr:       true,
			wantWatchable: false,
		},
		{
			name:    "unregistered config API scheme fails closed",
			reader:  fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build(),
			wantErr: true,
		},
		{
			name:          "transient apiserver error falls back to intermediate with watcher",
			reader:        errorReader{err: apierrors.NewServiceUnavailable("apiserver restarting")},
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
			wantFallback:  true,
		},
		{
			name:          "context deadline during load falls back to intermediate with watcher",
			reader:        errorReader{err: context.DeadlineExceeded},
			wantMin:       tls.VersionTLS12,
			wantCiphers:   true,
			wantProtos:    []string{http1Proto},
			wantWatchable: true,
			wantFallback:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := tt.reader
			if reader == nil {
				reader = apiServerClient(t, tt.profile, tt.adherence)
			}

			opts, result, err := managerTLSOptions(context.Background(), reader, tt.enableHTTP2)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if result.UsedFallback {
					t.Fatal("unexpected fallback after an unregistered API type error")
				}

				return
			}
			if err != nil {
				t.Fatalf("managerTLSOptions() error = %v", err)
			}

			cfg := &tls.Config{}
			for _, opt := range opts {
				opt(cfg)
			}

			if cfg.MinVersion != tt.wantMin {
				t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tt.wantMin)
			}
			if !tt.wantCiphers && !tt.wantOldCiphers && len(cfg.CipherSuites) != 0 {
				t.Errorf("CipherSuites = %v, want none", cfg.CipherSuites)
			}
			if tt.wantCiphers {
				assertECDHEAEAD(t, cfg.CipherSuites)
			}
			if tt.wantOldCiphers {
				assertOldProfileSuites(t, cfg.CipherSuites)
			}
			if !reflect.DeepEqual(cfg.NextProtos, tt.wantProtos) {
				t.Errorf("NextProtos = %v, want %v", cfg.NextProtos, tt.wantProtos)
			}
			if result.Watchable != tt.wantWatchable {
				t.Errorf("Watchable = %v, want %v", result.Watchable, tt.wantWatchable)
			}
			if result.UsedFallback != tt.wantFallback {
				t.Errorf("UsedFallback = %v, want %v", result.UsedFallback, tt.wantFallback)
			}
		})
	}
}

// assertOldProfileSuites checks the honoring contract for the Old profile:
// the profile's own suites (including CBC entries) instead of the hardened
// ECDHE-AEAD fallback set.
func assertOldProfileSuites(t *testing.T, suites []uint16) {
	t.Helper()

	if len(suites) == 0 {
		t.Fatal("expected the Old profile's cipher suites")
	}
	if !slices.Contains(suites, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA) {
		t.Error("expected Old's TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA among the suites")
	}
}

// assertECDHEAEAD checks the hardened fallback contract: TLS 1.2 suites
// restricted to ECDHE with AEAD (AES-GCM or CHACHA20).
func assertECDHEAEAD(t *testing.T, suites []uint16) {
	t.Helper()

	if len(suites) == 0 {
		t.Fatal("expected cipher suites")
	}

	for _, suite := range suites {
		name := tls.CipherSuiteName(suite)
		if !strings.Contains(name, "ECDHE") ||
			(!strings.Contains(name, "GCM") && !strings.Contains(name, "CHACHA20")) {
			t.Errorf("cipher suite %s is not ECDHE AEAD", name)
		}
	}
}

func TestConfigFromProfileMapping(t *testing.T) {
	tests := []struct {
		name         string
		spec         configv1.TLSProfileSpec
		wantMin      uint16
		wantCiphers  bool
		wantCustomID *uint16
	}{
		{
			name:        "old profile keeps its minimum version and mixes cipher families",
			spec:        *configv1.TLSProfiles[configv1.TLSProfileOldType],
			wantMin:     tls.VersionTLS10,
			wantCiphers: true,
		},
		{
			name:        "intermediate profile maps to ECDHE AEAD suites",
			spec:        *configv1.TLSProfiles[configv1.TLSProfileIntermediateType],
			wantMin:     tls.VersionTLS12,
			wantCiphers: true,
		},
		{
			name:        "modern profile maps to TLS 1.3 without cipher suites",
			spec:        *configv1.TLSProfiles[configv1.TLSProfileModernType],
			wantMin:     tls.VersionTLS13,
			wantCiphers: false,
		},
		{
			name:        "empty spec is floored to TLS 1.2",
			spec:        configv1.TLSProfileSpec{},
			wantMin:     tls.VersionTLS12,
			wantCiphers: false,
		},
		{
			name: "custom spec maps OpenSSL cipher names",
			spec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
			},
			wantMin:      tls.VersionTLS12,
			wantCiphers:  true,
			wantCustomID: &[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}[0],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsConfigFn, _, err := tlspkg.ConfigFromProfileWithError(tt.spec)
			if err != nil {
				t.Fatalf("ConfigFromProfileWithError() error = %v", err)
			}

			cfg := &tls.Config{}
			tlsConfigFn(cfg)

			if cfg.MinVersion != tt.wantMin {
				t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tt.wantMin)
			}
			if !tt.wantCiphers && len(cfg.CipherSuites) != 0 {
				t.Errorf("CipherSuites = %v, want none", cfg.CipherSuites)
			}
			if tt.wantCustomID != nil {
				want := []uint16{*tt.wantCustomID}
				if !reflect.DeepEqual(cfg.CipherSuites, want) {
					t.Errorf("CipherSuites = %v, want %v", cfg.CipherSuites, want)
				}
			}
		})
	}
}

func TestTLSProfileWatcherReloadsOnAdherenceOnlyChange(t *testing.T) {
	load := tlspkg.LoadResult{
		Spec:            *configv1.TLSProfiles[configv1.TLSProfileIntermediateType],
		AdherencePolicy: configv1.TLSAdherencePolicyLegacyAdheringComponentsOnly,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := newTLSProfileWatcher(fake.NewClientBuilder().Build(), load, cancel)

	if !reflect.DeepEqual(watcher.InitialTLSProfileSpec, load.Spec) {
		t.Error("watcher not seeded with the bootstrap profile spec")
	}
	if watcher.InitialTLSAdherencePolicy != load.AdherencePolicy {
		t.Error("watcher not seeded with the bootstrap adherence policy")
	}

	// An adherence-only change (profile spec unchanged) must trigger a reload.
	watcher.OnAdherencePolicyChange(ctx, load.AdherencePolicy, configv1.TLSAdherencePolicyStrictAllComponents)
	if ctx.Err() == nil {
		t.Error("adherence-only change did not trigger a reload")
	}
}

func TestTLSProfileWatcherReloadsOnProfileChange(t *testing.T) {
	load := tlspkg.LoadResult{
		Spec:            *configv1.TLSProfiles[configv1.TLSProfileIntermediateType],
		AdherencePolicy: configv1.TLSAdherencePolicyStrictAllComponents,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher := newTLSProfileWatcher(fake.NewClientBuilder().Build(), load, cancel)
	watcher.OnProfileChange(ctx, load.Spec, *configv1.TLSProfiles[configv1.TLSProfileModernType])
	if ctx.Err() == nil {
		t.Error("profile change did not trigger a reload")
	}
}
