package identityv1_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	gcApp    = "enum:platform.identity.v1.HandoffApp"
	gcResult = "message:platform.identity.v1.AuthResult"
)

// Scenario: Hand-off RPCs and messages exist
func TestGapsContract_HandoffRPCs(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	sd := fd.Services().ByName("AuthService")
	if sd == nil {
		t.Fatalf("AuthService missing")
	}
	for rpc, io := range map[string][2]string{
		"CreateHandoffCode": {"CreateHandoffCodeRequest", "CreateHandoffCodeResponse"},
		"RedeemHandoffCode": {"RedeemHandoffCodeRequest", "RedeemHandoffCodeResponse"},
		"PeekHandoffCode":   {"PeekHandoffCodeRequest", "PeekHandoffCodeResponse"},
	} {
		m := sd.Methods().ByName(protoreflect.Name(rpc))
		if m == nil {
			t.Errorf("AuthService.%s missing", rpc)
			continue
		}
		if string(m.Input().Name()) != io[0] || string(m.Output().Name()) != io[1] {
			t.Errorf("AuthService.%s = (%s) returns (%s), want (%s) returns (%s)", rpc, m.Input().Name(), m.Output().Name(), io[0], io[1])
		}
	}
	tcShape(t, fd, "CreateHandoffCodeRequest", map[string]string{"target_app": "1 " + gcApp})
	tcShape(t, fd, "CreateHandoffCodeResponse", map[string]string{"code": "1 string", "expires_at": "2 " + idTS})
	tcShape(t, fd, "RedeemHandoffCodeRequest", map[string]string{"code": "1 string", "target_app": "2 " + gcApp})
	tcShape(t, fd, "RedeemHandoffCodeResponse", map[string]string{"result": "1 " + gcResult})
	tcShape(t, fd, "PeekHandoffCodeRequest", map[string]string{"code": "1 string", "target_app": "2 " + gcApp})
	tcShape(t, fd, "PeekHandoffCodeResponse", map[string]string{"username": "1 string", "expires_at": "2 " + idTS})
}

// Scenario: BecomeSeller and event exist
func TestGapsContract_BecomeSellerAndEvent(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	sd := fd.Services().ByName("AuthService")
	if sd == nil {
		t.Fatalf("AuthService missing")
	}
	m := sd.Methods().ByName("BecomeSeller")
	if m == nil {
		t.Errorf("AuthService.BecomeSeller missing")
	} else if string(m.Input().Name()) != "BecomeSellerRequest" || string(m.Output().Name()) != "BecomeSellerResponse" {
		t.Errorf("AuthService.BecomeSeller = (%s) returns (%s)", m.Input().Name(), m.Output().Name())
	}
	tcShape(t, fd, "BecomeSellerRequest", map[string]string{"terms_version": "1 string"})
	tcShape(t, fd, "BecomeSellerResponse", map[string]string{"result": "1 " + gcResult})
	tcEnum(t, fd, "HandoffApp", map[string]int{
		"HANDOFF_APP_UNSPECIFIED": 0, "HANDOFF_APP_BUYER": 1, "HANDOFF_APP_SELLER": 2,
	})
	tcShape(t, fd, "UserRoleGranted", map[string]string{
		"user_id": "1 string", "role": "2 string", "granted_at": "3 " + idTS,
	})
}

// Scenario: Unspecified target is the zero value
func TestGapsContract_UnspecifiedTargetIsZero(t *testing.T) {
	fd, _ := tcFile(t, idProto)
	md := tcMsg(t, fd, "CreateHandoffCodeRequest")
	f := tcFld(t, md, "target_app")
	m := tcNew(t, md)
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(b) != 0 {
		t.Errorf("empty CreateHandoffCodeRequest wire size = %d, want 0", len(b))
	}
	got := tcRound(t, m)
	if n := got.Get(f).Enum(); n != 0 {
		t.Errorf("target_app decoded as %d, want 0 (HANDOFF_APP_UNSPECIFIED)", n)
	}
	if f.Enum().Values().ByNumber(0) == nil || f.Enum().Values().ByNumber(0).Name() != "HANDOFF_APP_UNSPECIFIED" {
		t.Errorf("enum value 0 of HandoffApp is not HANDOFF_APP_UNSPECIFIED")
	}
}

const gcbProtoDir = "../../.."

func gcbRun(t *testing.T, dir string, name string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if err == nil {
		return out.String(), 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.ExitCode()
	}
	t.Fatalf("harness: run %s: %v", name, err)
	return "", -1
}

func gcbCopyModule(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			if rel == "gen" || rel == "node_modules" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if rel != "buf.yaml" && !strings.HasPrefix(rel, "platform/identity/") && !strings.HasPrefix(rel, "platform/common/") {
			return nil
		}
		if rel != "buf.yaml" && !strings.HasSuffix(rel, ".proto") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatalf("harness: copy module: %v", err)
	}
	return dst
}

func gcbBaseline(t *testing.T) string {
	t.Helper()
	var lastErr string
	for _, ref := range []string{"main", "origin/main"} {
		cmd := exec.Command("git", "archive", "--format=tar", ref, ".")
		cmd.Dir = gcbProtoDir
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			lastErr = errb.String()
			continue
		}
		dst := t.TempDir()
		tr := tar.NewReader(&out)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("harness: read archive: %v", err)
			}
			p := filepath.Join(dst, h.Name)
			if h.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(p, 0o755); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(tr)
			if err := os.WriteFile(p, b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return gcbCopyModule(t, dst)
	}
	t.Fatalf("harness: no main baseline available: %s", lastErr)
	return ""
}

// Scenario: Existing identity contract is untouched
func TestGapsContract_IdentityBreakingClean(t *testing.T) {
	base := gcbBaseline(t)
	cur := gcbCopyModule(t, gcbProtoDir)
	out, rc := gcbRun(t, cur, "buf", "breaking", ".", "--against", base)
	if rc != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("buf breaking vs main: exit %d, findings:\n%s", rc, out)
	}
}

// Scenario: Existing identity contract is untouched (detector control: baseline is the real main proto)
func TestGapsContract_IdentityBaselineHasRefreshSession(t *testing.T) {
	base := gcbBaseline(t)
	b, err := os.ReadFile(filepath.Join(base, idProto))
	if err != nil {
		t.Fatalf("baseline missing identity.proto: %v", err)
	}
	if !strings.Contains(string(b), "rpc RefreshSession(RefreshSessionRequest) returns (RefreshSessionResponse);") {
		t.Fatal("baseline AuthService has no RefreshSession")
	}
}

// Scenario: Removing an existing identity RPC is rejected
func TestGapsContract_RemovedRPCDetected(t *testing.T) {
	orig := gcbCopyModule(t, gcbProtoDir)
	tmp := gcbCopyModule(t, gcbProtoDir)
	p := filepath.Join(tmp, idProto)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("harness: %v", err)
	}
	const rpc = "  rpc RefreshSession(RefreshSessionRequest) returns (RefreshSessionResponse);\n"
	if strings.Count(string(b), rpc) != 1 {
		t.Fatalf("harness: expected exactly one %q in current proto", rpc)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), rpc, "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	out, rc := gcbRun(t, tmp, "buf", "breaking", ".", "--against", orig)
	if rc == 0 {
		t.Fatalf("buf breaking accepted a removed RefreshSession; output:\n%s", out)
	}
	if !strings.Contains(out, "RefreshSession") {
		t.Errorf("finding does not name RefreshSession:\n%s", out)
	}
}

// Scenario: Removing an existing identity RPC is rejected (repo source never edited)
func TestGapsContract_RemovedRPCLeavesRepoSourceUntouched(t *testing.T) {
	before, err := os.ReadFile(filepath.Join(gcbProtoDir, idProto))
	if err != nil {
		t.Fatal(err)
	}
	TestGapsContract_RemovedRPCDetected(t)
	after, _ := os.ReadFile(filepath.Join(gcbProtoDir, idProto))
	if !bytes.Equal(before, after) {
		t.Fatal("repo identity.proto was modified by the test")
	}
}
