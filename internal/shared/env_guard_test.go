package shared

import (
	"reflect"
	"testing"
)

// No environment variable may name a field the interface writes.
//
// Derived, not restated: the forbidden set comes from the payload types the
// interface actually sends — SaveOptions carries FirewallOptions, SaveSettings
// carries NetworkSettings, SaveSystem carries SystemSettings. A test that
// merely listed forbidden keys would be edited in the same commit that added
// the offending variable, which is no protection at all.
//
// What it protects against is concrete: acceptance.duration looks like
// deployment. It is not. If it were settable from the environment, an operator
// would change the acceptance window in the interface, be told it was saved,
// and find the old value back after the next restart.
func TestNoEnvVarTargetsARuleField(t *testing.T) {
	forbidden := map[string]string{} // toml key -> the type it came from
	for _, payload := range []struct {
		name string
		typ  reflect.Type
	}{
		{"FirewallOptions", reflect.TypeOf(FirewallOptions{})},
		{"AcceptanceConfig", reflect.TypeOf(AcceptanceConfig{})},
		{"IPv6Config", reflect.TypeOf(IPv6Config{})},
		{"DockerConfig", reflect.TypeOf(DockerConfig{})},
		{"RoutingConfig", reflect.TypeOf(RoutingConfig{})},
	} {
		for _, k := range tomlKeys(payload.typ) {
			forbidden[k] = payload.name
		}
	}
	if len(forbidden) == 0 {
		t.Fatal("derived no forbidden keys; tomlKeys or the payload types changed shape")
	}

	for _, v := range CoreEnvVars {
		if from, bad := forbidden[v.TOMLKey]; bad {
			t.Errorf("%s targets %q, which the interface writes through %s\n"+
				"  an operator would press Save, be told it was saved, and find the "+
				"old value back after the next restart", v.Name, v.TOMLKey, from)
		}
	}
	for _, v := range WebEnvVars {
		if from, bad := forbidden[v.TOMLKey]; bad {
			t.Errorf("%s targets %q, which the interface writes through %s\n"+
				"  an operator would press Save, be told it was saved, and find the "+
				"old value back after the next restart", v.Name, v.TOMLKey, from)
		}
	}
}

// Every entry names a key that exists, so a typo cannot produce a variable that
// is documented, tested against the rule, and wired to nothing.
func TestEveryEnvVarNamesARealTOMLKey(t *testing.T) {
	core := map[string]bool{}
	for _, k := range tomlKeys(reflect.TypeOf(CoreConfig{})) {
		core[k] = true
	}
	for _, v := range CoreEnvVars {
		if !core[v.TOMLKey] {
			t.Errorf("%s names %q, which is not a key of CoreConfig", v.Name, v.TOMLKey)
		}
	}

	web := map[string]bool{}
	for _, k := range tomlKeys(reflect.TypeOf(WebConfig{})) {
		web[k] = true
	}
	// tls.cert and tls.key are nested one level down.
	for _, k := range tomlKeys(reflect.TypeOf(TLSConfig{})) {
		web["tls."+k] = true
	}
	for _, v := range WebEnvVars {
		if !web[v.TOMLKey] {
			t.Errorf("%s names %q, which is not a key of WebConfig", v.Name, v.TOMLKey)
		}
	}
}
