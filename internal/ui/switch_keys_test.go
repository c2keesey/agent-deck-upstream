package ui

import "testing"

func TestResolvedSwitchByte_Default(t *testing.T) {
	// Local fork: switch_session is remapped to Ctrl+W (upstream defaults it to
	// Ctrl+S, which is XOFF / what some terminals send for Cmd+Right). The
	// MRU-ordered switcher replaces the old blind MRU cursor-cycle on Ctrl+W.
	if got := ResolvedSwitchByte(nil); got != 'w'-'a'+1 {
		t.Errorf("default switch byte = %#x, want Ctrl+W (0x17)", got)
	}
}

func TestResolvedSwitchByte_OptInEnablesCtrlS(t *testing.T) {
	// Overriding the binding to Ctrl+S restores the upstream byte (user accepts
	// the collision with the attached program).
	if got := ResolvedSwitchByte(map[string]string{"switch_session": "ctrl+s"}); got != 0x13 {
		t.Errorf("override ctrl+s switch byte = %#x, want 0x13", got)
	}
}

func TestSwitchSessionBoundByDefault(t *testing.T) {
	// Local fork divergence from upstream: upstream ships switch_session UNBOUND
	// (opt-in Ctrl+S); the fork remaps it to Ctrl+W and keeps it BOUND by default
	// so the MRU-ordered switcher works out of the box. The canonical dispatch
	// token is therefore "ctrl+w" (the fork's home-screen `case "ctrl+w"` arm).
	bindings := resolveHotkeys(nil)
	if key, ok := bindings[hotkeySwitchSession]; !ok || key != "ctrl+w" {
		t.Errorf("switch_session = %q (ok=%v), want ctrl+w bound by default", key, ok)
	}
	lookup, _ := buildHotkeyLookup(bindings)
	if got := lookup["ctrl+w"]; got != "ctrl+w" {
		t.Errorf("ctrl+w canonical = %q, want ctrl+w dispatch token", got)
	}
}

func TestSwitchSessionOptInDispatch(t *testing.T) {
	// Overriding the key normalizes the chord back to the fork's canonical
	// dispatch token (ctrl+w) so the home-screen switcher arm still fires.
	bindings := resolveHotkeys(map[string]string{"switch_session": "ctrl+o"})
	if bindings[hotkeySwitchSession] != "ctrl+o" {
		t.Fatalf("switch_session = %q, want ctrl+o", bindings[hotkeySwitchSession])
	}
	lookup, _ := buildHotkeyLookup(bindings)
	if got := lookup["ctrl+o"]; got != "ctrl+w" {
		t.Errorf("ctrl+o canonical = %q, want ctrl+w dispatch token", got)
	}
}

func TestResolvedSwitchByte_Override(t *testing.T) {
	if got := ResolvedSwitchByte(map[string]string{"switch_session": "ctrl+x"}); got != 'x'-'a'+1 {
		t.Errorf("overridden switch byte = %#x, want ctrl+x", got)
	}
}

func TestResolvedSwitchByte_UnboundOrNonCtrl(t *testing.T) {
	// Unbound -> 0 (disabled).
	if got := ResolvedSwitchByte(map[string]string{"switch_session": ""}); got != 0 {
		t.Errorf("unbound switch byte = %#x, want 0", got)
	}
	// A non-ctrl binding has no portable byte -> 0.
	if got := ResolvedSwitchByte(map[string]string{"switch_session": "ctrl+tab"}); got != 0 {
		t.Errorf("ctrl+tab switch byte = %#x, want 0 (no legacy byte)", got)
	}
}

func TestCtrlByteFromBinding(t *testing.T) {
	cases := map[string]byte{
		"ctrl+s":         0x13,
		"ctrl+a":         0x01,
		"CTRL+S":         0x13, // case-insensitive
		"ctrl+]":         0x1D,
		"ctrl+tab":       0, // no legacy byte
		"ctrl+shift+tab": 0,
		"tab":            0,
		"s":              0,
		"":               0,
	}
	for binding, want := range cases {
		if got := ctrlByteFromBinding(binding); got != want {
			t.Errorf("ctrlByteFromBinding(%q) = %#x, want %#x", binding, got, want)
		}
	}
}
