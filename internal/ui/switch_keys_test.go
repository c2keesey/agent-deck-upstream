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
