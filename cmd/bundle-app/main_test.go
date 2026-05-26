package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBundle_LayoutAndPlist exercises the bundling logic end-to-end
// against a stub binary. Skips codesign so the test stays cross-platform.
func TestBundle_LayoutAndPlist(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "stub")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "Stub.app")

	err := bundle(binPath, out, plistData{
		Name:       "Stub",
		BundleID:   "dev.local.stub",
		Version:    "1.2.3",
		CalDesc:    "test cal",
		RemDesc:    "test rem",
		Background: true,
	}, false)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}

	// Layout
	for _, p := range []string{
		out + "/Contents/Info.plist",
		out + "/Contents/MacOS/Stub",
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}

	// Binary preserved
	got, err := os.ReadFile(out + "/Contents/MacOS/Stub")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "echo hi") {
		t.Errorf("bundled binary content unexpected: %q", got)
	}

	// Plist has expected keys
	plist, err := os.ReadFile(out + "/Contents/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<key>CFBundleIdentifier</key><string>dev.local.stub</string>",
		"<key>CFBundleName</key><string>Stub</string>",
		"<key>CFBundleExecutable</key><string>Stub</string>",
		"<key>CFBundleShortVersionString</key><string>1.2.3</string>",
		"<key>LSUIElement</key><true/>",
		"<key>NSCalendarsUsageDescription</key><string>test cal</string>",
		"<key>NSRemindersUsageDescription</key><string>test rem</string>",
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("plist missing %q\n---\n%s", want, plist)
		}
	}
}

func TestBundle_IdempotentOverwrite(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "stub")
	if err := os.WriteFile(binPath, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "Stub.app")

	// Pre-populate the output with junk to confirm RemoveAll cleans it.
	if err := os.MkdirAll(filepath.Join(out, "junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "junk", "leftover"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := bundle(binPath, out, plistData{Name: "Stub"}, false); err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "junk")); !os.IsNotExist(err) {
		t.Errorf("leftover from prior bundle survived; err = %v", err)
	}
}

func TestBundle_SuppressesEmptyUsageStrings(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "stub")
	if err := os.WriteFile(binPath, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "Stub.app")
	if err := bundle(binPath, out, plistData{Name: "Stub"}, false); err != nil {
		t.Fatal(err)
	}
	plist, err := os.ReadFile(out + "/Contents/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plist), "NSCalendarsUsageDescription") {
		t.Errorf("empty cal-desc should not emit the key:\n%s", plist)
	}
	if strings.Contains(string(plist), "NSRemindersUsageDescription") {
		t.Errorf("empty rem-desc should not emit the key:\n%s", plist)
	}
}

func TestBundle_XMLEscapesUserInput(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "stub")
	if err := os.WriteFile(binPath, []byte("ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "Stub.app")
	err := bundle(binPath, out, plistData{
		Name:    `evil"<&>name`,
		CalDesc: `<script>alert(1)</script>`,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	plist, err := os.ReadFile(out + "/Contents/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	s := string(plist)
	if strings.Contains(s, "<script>") {
		t.Errorf("user input not escaped:\n%s", s)
	}
	if !strings.Contains(s, "&lt;script&gt;") {
		t.Errorf("expected XML-escaped script tag in:\n%s", s)
	}
}

func TestDeriveName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/tmp/eventkit-server", "Eventkit-server"},
		{"./build/myapp", "Myapp"},
		{"foo", "Foo"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := deriveName(tt.in); got != tt.want {
				t.Errorf("deriveName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBundle_RejectsMissingBinary(t *testing.T) {
	err := bundle("/tmp/does-not-exist-12345", filepath.Join(t.TempDir(), "Out.app"), plistData{Name: "Out"}, false)
	if err == nil {
		t.Errorf("expected error for missing bin")
	}
}

func TestBundle_RejectsDirAsBin(t *testing.T) {
	dir := t.TempDir()
	err := bundle(dir, filepath.Join(t.TempDir(), "Out.app"), plistData{Name: "Out"}, false)
	if err == nil {
		t.Errorf("expected error when --bin is a directory")
	}
}
