package referralv1_test

// Tests for change q7-referral-welcome-contract (capability q7-referral-welcome-contract).
// Stdlib only: the schema of the identity, referral, promotion and order protos is read
// from one `buf build -o -#format=json` (a FileDescriptorSet as JSON); a tiny
// schema-driven proto3 wire codec proves round-trip and old-reader compatibility
// without generated code. Comment text is whitespace-normalised.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type fieldDesc struct {
	Name           string `json:"name"`
	Number         int    `json:"number"`
	Type           string `json:"type"`
	TypeName       string `json:"typeName"`
	Label          string `json:"label"`
	Proto3Optional bool   `json:"proto3Optional"`
}

type msgDesc struct {
	Name  string      `json:"name"`
	Field []fieldDesc `json:"field"`
}

type enumValue struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type enumDesc struct {
	Name  string      `json:"name"`
	Value []enumValue `json:"value"`
}

type methodDesc struct {
	Name       string `json:"name"`
	InputType  string `json:"inputType"`
	OutputType string `json:"outputType"`
}

type svcDesc struct {
	Name   string       `json:"name"`
	Method []methodDesc `json:"method"`
}

type location struct {
	Path            []int  `json:"path"`
	LeadingComments string `json:"leadingComments"`
	TrailingComment string `json:"trailingComments"`
}

type fileDesc struct {
	Name           string     `json:"name"`
	Package        string     `json:"package"`
	MessageType    []msgDesc  `json:"messageType"`
	EnumType       []enumDesc `json:"enumType"`
	Service        []svcDesc  `json:"service"`
	SourceCodeInfo struct {
		Location []location `json:"location"`
	} `json:"sourceCodeInfo"`
}

const (
	identityFile  = "platform/identity/v1/identity.proto"
	referralFile  = "platform/referral/v1/referral.proto"
	promotionFile = "platform/promotion/v1/promotion.proto"
	orderFile     = "platform/order/v1/order.proto"
)

var (
	loadOnce sync.Once
	loaded   map[string]fileDesc
	loadErr  error
)

func loadAll(t *testing.T) map[string]fileDesc {
	t.Helper()
	loadOnce.Do(func() {
		cmd := exec.Command("buf", "build", "-o", "-#format=json")
		cmd.Dir = "../../.."
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			loadErr = fmt.Errorf("buf build failed (harness): %v\n%s", err, errb.String())
			return
		}
		var set struct {
			File []fileDesc `json:"file"`
		}
		if err := json.Unmarshal(out.Bytes(), &set); err != nil {
			loadErr = fmt.Errorf("decode descriptor json: %v", err)
			return
		}
		loaded = map[string]fileDesc{}
		for _, f := range set.File {
			loaded[f.Name] = f
		}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	return loaded
}

func loadFile(t *testing.T, name string) fileDesc {
	t.Helper()
	f, ok := loadAll(t)[name]
	if !ok {
		t.Fatalf("%s not in descriptor set", name)
	}
	return f
}

func (f fileDesc) pkgPrefix() string { return "." + f.Package + "." }

func (f fileDesc) message(name string) (msgDesc, int, bool) {
	for i, m := range f.MessageType {
		if m.Name == name {
			return m, i, true
		}
	}
	return msgDesc{}, -1, false
}

func (f fileDesc) mustMessage(t *testing.T, name string) (msgDesc, int) {
	t.Helper()
	m, i, ok := f.message(name)
	if !ok {
		t.Fatalf("message %s%s missing from descriptor", f.pkgPrefix(), name)
	}
	return m, i
}

func (f fileDesc) enum(name string) (enumDesc, int, bool) {
	for i, e := range f.EnumType {
		if e.Name == name {
			return e, i, true
		}
	}
	return enumDesc{}, -1, false
}

func (m msgDesc) fieldIndex(name string) (fieldDesc, int, bool) {
	for i, fd := range m.Field {
		if fd.Name == name {
			return fd, i, true
		}
	}
	return fieldDesc{}, -1, false
}

func (f fileDesc) service(name string) (svcDesc, int, bool) {
	for i, s := range f.Service {
		if s.Name == name {
			return s, i, true
		}
	}
	return svcDesc{}, -1, false
}

func (f fileDesc) comment(path ...int) (string, bool) {
	for _, l := range f.SourceCodeInfo.Location {
		if reflect.DeepEqual(l.Path, path) {
			return l.LeadingComments + l.TrailingComment, true
		}
	}
	return "", false
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

func checkKeys(comment string, ok bool, subs ...string) error {
	if !ok {
		return fmt.Errorf("no comment found on element")
	}
	c := norm(comment)
	var missing []string
	for _, s := range subs {
		if !strings.Contains(c, norm(s)) {
			missing = append(missing, fmt.Sprintf("%q", s))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("comment missing %s; got:\n%s", strings.Join(missing, ", "), c)
	}
	return nil
}

func (f fileDesc) rpcComment(t *testing.T, svc, rpc string) (string, bool) {
	t.Helper()
	s, si, ok := f.service(svc)
	if !ok {
		t.Errorf("service %s missing", svc)
		return "", false
	}
	for mi, m := range s.Method {
		if m.Name == rpc {
			return f.comment(6, si, 2, mi)
		}
	}
	t.Errorf("rpc %s.%s missing", svc, rpc)
	return "", false
}

func (f fileDesc) msgComment(t *testing.T, msg string) (string, bool) {
	t.Helper()
	_, mi, ok := f.message(msg)
	if !ok {
		t.Errorf("message %s missing", msg)
		return "", false
	}
	return f.comment(4, mi)
}

func (f fileDesc) fieldComment(t *testing.T, msg, field string) (string, bool) {
	t.Helper()
	m, mi, ok := f.message(msg)
	if !ok {
		t.Errorf("message %s missing", msg)
		return "", false
	}
	_, fi, ok := m.fieldIndex(field)
	if !ok {
		t.Errorf("field %s.%s missing", msg, field)
		return "", false
	}
	return f.comment(4, mi, 2, fi)
}

// ---- minimal schema-driven proto3 wire codec -------------------------------

func isVarint(t string) bool {
	switch t {
	case "TYPE_BOOL", "TYPE_INT32", "TYPE_INT64", "TYPE_UINT32", "TYPE_UINT64", "TYPE_ENUM":
		return true
	}
	return false
}

func encode(m msgDesc, vals map[string]any) []byte {
	fs := append([]fieldDesc(nil), m.Field...)
	sort.Slice(fs, func(i, j int) bool { return fs[i].Number < fs[j].Number })
	var b []byte
	for _, fd := range fs {
		v, ok := vals[fd.Name]
		if !ok {
			continue
		}
		switch {
		case isVarint(fd.Type):
			var u uint64
			switch x := v.(type) {
			case bool:
				if x {
					u = 1
				}
			case int64:
				u = uint64(x)
			}
			if u == 0 && !fd.Proto3Optional {
				continue
			}
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3)
			b = binary.AppendUvarint(b, u)
		case fd.Type == "TYPE_STRING":
			s := v.(string)
			if s == "" && !fd.Proto3Optional {
				continue
			}
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
			b = binary.AppendUvarint(b, uint64(len(s)))
			b = append(b, s...)
		case fd.Type == "TYPE_MESSAGE":
			raw := v.([]byte)
			b = binary.AppendUvarint(b, uint64(fd.Number)<<3|2)
			b = binary.AppendUvarint(b, uint64(len(raw)))
			b = append(b, raw...)
		}
	}
	return b
}

// decode parses b with the supplied field set; unknown fields are skipped.
// Message fields appear in the result only when present on the wire.
func decode(t *testing.T, fields []fieldDesc, b []byte) map[string]any {
	t.Helper()
	byNum := map[int]fieldDesc{}
	out := map[string]any{}
	for _, fd := range fields {
		byNum[fd.Number] = fd
		switch {
		case fd.Type == "TYPE_BOOL":
			out[fd.Name] = false
		case isVarint(fd.Type):
			out[fd.Name] = int64(0)
		case fd.Type == "TYPE_STRING":
			out[fd.Name] = ""
		}
	}
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			t.Fatalf("bad tag")
		}
		b = b[n:]
		num, wt := int(tag>>3), int(tag&7)
		fd, known := byNum[num]
		switch wt {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				t.Fatalf("bad varint")
			}
			b = b[n:]
			if known {
				if fd.Type == "TYPE_BOOL" {
					out[fd.Name] = v != 0
				} else {
					out[fd.Name] = int64(v)
				}
			}
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || int(l) > len(b)-n {
				t.Fatalf("bad length")
			}
			raw := b[n : n+int(l)]
			b = b[n+int(l):]
			if known && fd.Type == "TYPE_STRING" {
				out[fd.Name] = string(raw)
			}
			if known && fd.Type == "TYPE_MESSAGE" {
				out[fd.Name] = append([]byte(nil), raw...)
			}
		case 1:
			b = b[8:]
		case 5:
			b = b[4:]
		default:
			t.Fatalf("unsupported wire type %d", wt)
		}
	}
	return out
}

func wireNumbers(t *testing.T, b []byte) []int {
	t.Helper()
	var nums []int
	for len(b) > 0 {
		tag, n := binary.Uvarint(b)
		if n <= 0 {
			t.Fatalf("bad tag")
		}
		b = b[n:]
		switch tag & 7 {
		case 0:
			_, n := binary.Uvarint(b)
			b = b[n:]
		case 2:
			l, n := binary.Uvarint(b)
			b = b[n+int(l):]
		default:
			t.Fatalf("unexpected wire type")
		}
		nums = append(nums, int(tag>>3))
	}
	return nums
}

func upTo(fs []fieldDesc, max int) []fieldDesc {
	var o []fieldDesc
	for _, fd := range fs {
		if fd.Number <= max {
			o = append(o, fd)
		}
	}
	return o
}

func timestampBytes(secs int64) []byte {
	b := binary.AppendUvarint(nil, 1<<3)
	return binary.AppendUvarint(b, uint64(secs))
}

// ---- descriptor shape helpers ----------------------------------------------

func fieldShape(fd fieldDesc) string {
	s := fmt.Sprintf("%d %s", fd.Number, fd.Type)
	if fd.TypeName != "" {
		s += " " + fd.TypeName
	}
	if fd.Label == "LABEL_REPEATED" {
		s += " repeated"
	}
	if fd.Proto3Optional {
		s += " optional"
	}
	return s
}

func fields(m msgDesc) map[string]string {
	got := map[string]string{}
	for _, fd := range m.Field {
		got[fd.Name] = fieldShape(fd)
	}
	return got
}

func assertMessageFields(t *testing.T, f fileDesc, want map[string]map[string]string) {
	t.Helper()
	for name, w := range want {
		m, _, ok := f.message(name)
		if !ok {
			t.Errorf("message %s missing", name)
			continue
		}
		if g := fields(m); !reflect.DeepEqual(g, w) {
			t.Errorf("%s fields = %v, want exactly %v", name, g, w)
		}
	}
}

func assertEnum(t *testing.T, f fileDesc, name string, want map[string]int) {
	t.Helper()
	e, _, ok := f.enum(name)
	if !ok {
		t.Errorf("enum %s missing", name)
		return
	}
	got := map[string]int{}
	for _, v := range e.Value {
		got[v.Name] = v.Number
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enum %s = %v, want exactly %v", name, got, want)
	}
}

func assertRPCs(t *testing.T, f fileDesc, svc string, existing []string, added ...string) {
	t.Helper()
	s, _, ok := f.service(svc)
	if !ok {
		t.Fatalf("service %s missing", svc)
	}
	got := map[string][2]string{}
	for _, m := range s.Method {
		got[m.Name] = [2]string{strings.TrimPrefix(m.InputType, f.pkgPrefix()), strings.TrimPrefix(m.OutputType, f.pkgPrefix())}
	}
	want := map[string][2]string{}
	for _, n := range append(append([]string(nil), existing...), added...) {
		want[n] = [2]string{n + "Request", n + "Response"}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s methods = %v, want exactly %v", svc, got, want)
	}
}

// ---- identity ---------------------------------------------------------------

var authExisting = []string{"Register", "Login", "ChangePassword", "RequestPasswordReset", "ResetPassword",
	"RefreshSession", "CreateHandoffCode", "RedeemHandoffCode", "PeekHandoffCode", "BecomeSeller"}

const (
	tsType      = "TYPE_MESSAGE .google.protobuf.Timestamp"
	str         = "TYPE_STRING"
	i64         = "TYPE_INT64"
	registerKey = "referral_code"
)

// Scenario: Q7 referral welcome contract: RegisterRequest carries an optional referral code
func TestQ7_RegisterRequestCarriesAnOptionalReferralCode(t *testing.T) {
	f := loadFile(t, identityFile)
	assertMessageFields(t, f, map[string]map[string]string{
		"RegisterRequest": {
			"username": "1 " + str, "password": "2 " + str, "role": "3 " + str, registerKey: "4 " + str,
		},
	})
}

// Scenario: Q7 referral welcome contract: UserRegistered message is fixed
func TestQ7_UserRegisteredMessageIsFixed(t *testing.T) {
	f := loadFile(t, identityFile)
	assertMessageFields(t, f, map[string]map[string]string{
		"UserRegistered": {"user_id": "1 " + str, "referral_code": "2 " + str, "registered_at": "3 " + tsType},
	})
}

// Scenario: Q7 referral welcome contract: UserRegistered without a code round-trips
func TestQ7_UserRegisteredWithoutACodeRoundTrips(t *testing.T) {
	f := loadFile(t, identityFile)
	m, _ := f.mustMessage(t, "UserRegistered")
	ts := timestampBytes(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC).Unix())
	in := map[string]any{"user_id": "01JABCDEFGHJKMNPQRSTVWXYZ0", "referral_code": "", "registered_at": ts}
	b := encode(m, in)
	for _, n := range wireNumbers(t, b) {
		if n == 2 {
			t.Errorf("empty referral_code is present on the wire")
		}
	}
	got := decode(t, m.Field, b)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip = %v, want %v", got, in)
	}
}

// Scenario: Q7 referral welcome contract: Old readers skip the referral code
func TestQ7_OldReadersSkipTheReferralCode(t *testing.T) {
	f := loadFile(t, identityFile)
	m, _ := f.mustMessage(t, "RegisterRequest")
	if _, _, ok := m.fieldIndex(registerKey); !ok {
		t.Fatal("RegisterRequest.referral_code missing")
	}
	in := map[string]any{"username": "alice", "password": "pw", "role": "buyer", registerKey: "ABC123"}
	got := decode(t, upTo(m.Field, 3), encode(m, in))
	want := map[string]any{"username": "alice", "password": "pw", "role": "buyer"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("old-schema decode = %v, want %v", got, want)
	}
}

// Scenario: Q7 referral welcome contract: UserRegistered semantics are documented
func TestQ7_UserRegisteredSemanticsAreDocumented(t *testing.T) {
	f := loadFile(t, identityFile)
	c, ok := f.fieldComment(t, "RegisterRequest", registerKey)
	if err := checkKeys(c, ok, "never fails"); err != nil {
		t.Errorf("RegisterRequest.referral_code: %v", err)
	}
	c, ok = f.msgComment(t, "UserRegistered")
	err := checkKeys(c, ok, "identity.events", "platform.identity.v1.UserRegistered", "same transaction",
		"idempotent", "internal-only", "never exposed at the gateway")
	if err != nil {
		t.Errorf("UserRegistered: %v", err)
	}
}

// Scenario: Q7 referral welcome contract: Missing never-fails note is caught
func TestQ7_MissingNeverFailsNoteIsCaught(t *testing.T) {
	fixture := "Optional referral code accepted on the public Register RPC."
	if err := checkKeys(fixture, true, "never fails"); err == nil {
		t.Fatal("helper accepted a comment lacking never fails")
	}
	if err := checkKeys("", false, "never fails"); err == nil {
		t.Fatal("helper accepted a missing comment")
	}
	if err := checkKeys("An unknown code never\n  fails Register.", true, "never fails"); err != nil {
		t.Fatalf("helper rejected a wrapped comment containing never fails: %v", err)
	}
}

// Scenario: Q7 referral welcome contract: GetMyAccount exists on AuthService
func TestQ7_GetMyAccountExistsOnAuthService(t *testing.T) {
	f := loadFile(t, identityFile)
	assertRPCs(t, f, "AuthService", authExisting, "GetMyAccount")
	assertMessageFields(t, f, map[string]map[string]string{
		"GetMyAccountRequest":  {},
		"GetMyAccountResponse": {"user_id": "1 " + str, "created_at": "2 " + tsType},
	})
}

// Scenario: Q7 referral welcome contract: GetMyAccount scope is documented
func TestQ7_GetMyAccountScopeIsDocumented(t *testing.T) {
	f := loadFile(t, identityFile)
	c, ok := f.rpcComment(t, "AuthService", "GetMyAccount")
	if err := checkKeys(c, ok, "principal-scoped", "own account", "no request field", "any signed-in user"); err != nil {
		t.Error(err)
	}
}

// Scenario: Q7 referral welcome contract: GetMyAccount response round-trips
func TestQ7_GetMyAccountResponseRoundTrips(t *testing.T) {
	f := loadFile(t, identityFile)
	m, _ := f.mustMessage(t, "GetMyAccountResponse")
	ts := timestampBytes(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC).Unix())
	in := map[string]any{"user_id": "u-1", "created_at": ts}
	if got := decode(t, m.Field, encode(m, in)); !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip = %v, want %v", got, in)
	}
	got := decode(t, m.Field, encode(m, map[string]any{"user_id": "u-1"}))
	if _, set := got["created_at"]; set {
		t.Errorf("unset created_at parsed as set: %v", got)
	}
	if got["user_id"] != "u-1" {
		t.Errorf("user_id = %v", got["user_id"])
	}
}

// Scenario: Q7 referral welcome contract: Missing principal-scope note is caught
func TestQ7_MissingPrincipalScopeNoteIsCaught(t *testing.T) {
	fixture := "Returns the account. Routed at the gateway."
	if err := checkKeys(fixture, true, "principal-scoped"); err == nil {
		t.Fatal("helper accepted a comment lacking principal-scoped")
	}
	if err := checkKeys("Is principal-scoped.", true, "principal-scoped"); err != nil {
		t.Fatalf("helper rejected a good comment: %v", err)
	}
}

// ---- referral ---------------------------------------------------------------

const (
	rejectEnum = "TYPE_ENUM .platform.referral.v1.ReferralRejectReason"
	statusEnum = "TYPE_ENUM .platform.referral.v1.ReferralRewardStatus"
)

// Scenario: Q7 referral welcome contract: Referral enums have fixed names and numbers
func TestQ7_ReferralEnumsHaveFixedNamesAndNumbers(t *testing.T) {
	f := loadFile(t, referralFile)
	assertEnum(t, f, "ReferralRejectReason", map[string]int{
		"REFERRAL_REJECT_REASON_UNSPECIFIED":     0,
		"REFERRAL_REJECT_REASON_UNKNOWN_CODE":    1,
		"REFERRAL_REJECT_REASON_SELF_REFERRAL":   2,
		"REFERRAL_REJECT_REASON_REFERRER_CAPPED": 3,
	})
	assertEnum(t, f, "ReferralRewardStatus", map[string]int{
		"REFERRAL_REWARD_STATUS_UNSPECIFIED": 0,
		"REFERRAL_REWARD_STATUS_PENDING":     1,
		"REFERRAL_REWARD_STATUS_RELEASED":    2,
		"REFERRAL_REWARD_STATUS_VOID":        3,
	})
}

// Scenario: Q7 referral welcome contract: Referral event messages have fixed fields
func TestQ7_ReferralEventMessagesHaveFixedFields(t *testing.T) {
	f := loadFile(t, referralFile)
	assertMessageFields(t, f, map[string]map[string]string{
		"ReferralAttributed": {"referee_id": "1 " + str, "referrer_id": "2 " + str, "attributed_at": "3 " + tsType},
		"ReferralRejected":   {"referee_id": "1 " + str, "reason": "2 " + rejectEnum},
		"ReferralRewardReleased": {
			"reward_id": "1 " + str, "referrer_id": "2 " + str, "referee_id": "3 " + str,
			"amount_xu": "4 " + i64, "released_at": "5 " + tsType,
		},
	})
}

// Scenario: Q7 referral welcome contract: Reward and summary additions are fixed
func TestQ7_RewardAndSummaryAdditionsAreFixed(t *testing.T) {
	f := loadFile(t, referralFile)
	assertMessageFields(t, f, map[string]map[string]string{
		"ReferralReward": {
			"id": "1 " + str, "amount": "2 " + i64, "reason": "3 " + str, "created_at": "4 " + tsType,
			"status": "5 " + statusEnum, "release_at": "6 " + tsType,
		},
		"GetMyReferralResponse": {
			"code": "1 " + str, "invited_count": "2 " + i64, "rewards_total": "3 " + i64,
			"pending_xu": "4 " + i64, "released_xu": "5 " + i64,
		},
	})
}

// Scenario: Q7 referral welcome contract: Released event round-trips with a large amount
func TestQ7_ReleasedEventRoundTripsWithALargeAmount(t *testing.T) {
	f := loadFile(t, referralFile)
	m, _ := f.mustMessage(t, "ReferralRewardReleased")
	ts := timestampBytes(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC).Unix())
	in := map[string]any{
		"reward_id": "r-1", "referrer_id": "u-1", "referee_id": "u-2",
		"amount_xu": int64(9223372036854775807), "released_at": ts,
	}
	if got := decode(t, m.Field, encode(m, in)); !reflect.DeepEqual(got, in) {
		t.Errorf("round-trip = %v, want %v", got, in)
	}
}

// Scenario: Q7 referral welcome contract: Rejected event round-trips and tolerates an unknown reason
func TestQ7_RejectedEventRoundTripsAndToleratesAnUnknownReason(t *testing.T) {
	f := loadFile(t, referralFile)
	m, _ := f.mustMessage(t, "ReferralRejected")
	e, _, ok := f.enum("ReferralRejectReason")
	if !ok {
		t.Fatal("enum ReferralRejectReason missing")
	}
	var capped int64 = -1
	for _, v := range e.Value {
		if v.Name == "REFERRAL_REJECT_REASON_REFERRER_CAPPED" {
			capped = int64(v.Number)
		}
	}
	in := map[string]any{"referee_id": "u-2", "reason": capped}
	if got := decode(t, m.Field, encode(m, in)); !reflect.DeepEqual(got, in) || capped != 3 {
		t.Errorf("round-trip = %v (capped=%d), want %v with 3", got, capped, in)
	}
	unknown := encode(m, map[string]any{"referee_id": "u-2", "reason": int64(99)})
	got := decode(t, m.Field, unknown)
	if got["reason"] != int64(99) {
		t.Errorf("unknown reason parsed as %v, want 99", got["reason"])
	}
}

// Scenario: Q7 referral welcome contract: Old readers skip the reward additions
func TestQ7_OldReadersSkipTheRewardAdditions(t *testing.T) {
	f := loadFile(t, referralFile)
	ts := timestampBytes(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC).Unix())
	rw, _ := f.mustMessage(t, "ReferralReward")
	for _, n := range []string{"status", "release_at"} {
		if _, _, ok := rw.fieldIndex(n); !ok {
			t.Fatalf("ReferralReward.%s missing", n)
		}
	}
	in := map[string]any{"id": "r-1", "amount": int64(5000), "reason": "referral", "created_at": ts,
		"status": int64(2), "release_at": ts}
	want := map[string]any{"id": "r-1", "amount": int64(5000), "reason": "referral", "created_at": ts}
	if got := decode(t, upTo(rw.Field, 4), encode(rw, in)); !reflect.DeepEqual(got, want) {
		t.Errorf("old ReferralReward decode = %v, want %v", got, want)
	}
	sm, _ := f.mustMessage(t, "GetMyReferralResponse")
	for _, n := range []string{"pending_xu", "released_xu"} {
		if _, _, ok := sm.fieldIndex(n); !ok {
			t.Fatalf("GetMyReferralResponse.%s missing", n)
		}
	}
	sin := map[string]any{"code": "ABC", "invited_count": int64(3), "rewards_total": int64(10),
		"pending_xu": int64(7), "released_xu": int64(10)}
	swant := map[string]any{"code": "ABC", "invited_count": int64(3), "rewards_total": int64(10)}
	if got := decode(t, upTo(sm.Field, 3), encode(sm, sin)); !reflect.DeepEqual(got, swant) {
		t.Errorf("old GetMyReferralResponse decode = %v, want %v", got, swant)
	}
}

var eventMessages = []string{"ReferralAttributed", "ReferralRejected", "ReferralRewardReleased"}

// Scenario: Q7 referral welcome contract: Referral event semantics are documented
func TestQ7_ReferralEventSemanticsAreDocumented(t *testing.T) {
	f := loadFile(t, referralFile)
	for _, msg := range eventMessages {
		c, ok := f.msgComment(t, msg)
		if err := checkKeys(c, ok, "referral.events", "key = referee", "platform.referral.v1."+msg); err != nil {
			t.Errorf("%s: %v", msg, err)
		}
	}
	for _, msg := range []string{"ReferralAttributed", "ReferralRejected"} {
		c, ok := f.msgComment(t, msg)
		if err := checkKeys(c, ok, "mutually exclusive"); err != nil {
			t.Errorf("%s: %v", msg, err)
		}
	}
	c, ok := f.msgComment(t, "ReferralRewardReleased")
	if err := checkKeys(c, ok, "once per reward"); err != nil {
		t.Errorf("ReferralRewardReleased: %v", err)
	}
	c, ok = f.msgComment(t, "ReferralReward")
	if err := checkKeys(c, ok, "xu"); err != nil {
		t.Errorf("ReferralReward: %v", err)
	}
	c, ok = f.msgComment(t, "GetMyReferralResponse")
	if err := checkKeys(c, ok, "xu", "equals released_xu"); err != nil {
		t.Errorf("GetMyReferralResponse: %v", err)
	}
	c, ok = f.rpcComment(t, "ReferralService", "RedeemReferral")
	if err := checkKeys(c, ok, "FAILED_PRECONDITION", "credits nothing", "168h", "team-referral"); err != nil {
		t.Errorf("RedeemReferral: %v", err)
	}
}

// Scenario: Q7 referral welcome contract: Referral events are documented as internal-only
func TestQ7_ReferralEventsAreDocumentedAsInternalOnly(t *testing.T) {
	f := loadFile(t, referralFile)
	for _, msg := range eventMessages {
		c, ok := f.msgComment(t, msg)
		if err := checkKeys(c, ok, "internal-only", "never exposed at the gateway"); err != nil {
			t.Errorf("%s: %v", msg, err)
		}
	}
}

// Scenario: Q7 referral welcome contract: Missing xu note is caught
func TestQ7_MissingXuNoteIsCaught(t *testing.T) {
	fixture := "Summary of the caller's referral programme standing."
	if err := checkKeys(fixture, true, "xu"); err == nil {
		t.Fatal("helper accepted a comment lacking xu")
	}
	if err := checkKeys("All three totals are xu.", true, "xu"); err != nil {
		t.Fatalf("helper rejected a good comment: %v", err)
	}
}

// ---- promotion --------------------------------------------------------------

var voucherExisting = []string{"CreateVoucher", "GetVoucher", "ListVouchers", "DeactivateVoucher", "UpdateVoucher",
	"ValidateAndReserve", "CommitReservation", "ReleaseReservation"}

const (
	kindEnum = "TYPE_ENUM .platform.promotion.v1.VoucherKind"
	scopeEnm = "TYPE_ENUM .platform.promotion.v1."
)

func voucherFields() map[string]string {
	return map[string]string{
		"id": "1 " + str, "code": "2 " + str, "scope": "3 " + scopeEnm + "VoucherScope", "seller_id": "4 " + str,
		"discount_type": "5 " + scopeEnm + "DiscountType", "discount_value": "6 " + i64, "min_spend": "7 " + i64,
		"max_discount": "8 " + i64, "quota": "9 " + i64, "used": "10 " + i64,
		"starts_at": "11 " + tsType, "ends_at": "12 " + tsType, "active": "13 TYPE_BOOL",
		"deactivated_at": "14 " + tsType,
	}
}

// Scenario: Q7 referral welcome contract: VoucherKind has fixed names and numbers
func TestQ7_VoucherKindHasFixedNamesAndNumbers(t *testing.T) {
	f := loadFile(t, promotionFile)
	assertEnum(t, f, "VoucherKind", map[string]int{
		"VOUCHER_KIND_UNSPECIFIED": 0, "VOUCHER_KIND_STANDARD": 1, "VOUCHER_KIND_NEW_USER": 2,
	})
}

// Scenario: Q7 referral welcome contract: Voucher user-bound fields have fixed numbers
func TestQ7_VoucherUserBoundFieldsHaveFixedNumbers(t *testing.T) {
	f := loadFile(t, promotionFile)
	vm, _ := f.mustMessage(t, "Voucher")
	// the pre-existing enum type names are not part of this contract; pin only number and wire type
	got := map[string]string{}
	for _, fd := range vm.Field {
		got[fd.Name] = fieldShape(fd)
	}
	wantV := map[string]string{}
	for k, v := range voucherFields() {
		wantV[k] = v
	}
	wantV["owner_user_id"] = "15 " + str
	wantV["kind"] = "16 " + kindEnum
	wantV["per_user_limit"] = "17 TYPE_INT32"
	wantV["new_user_only"] = "18 TYPE_BOOL"
	for _, n := range []string{"scope", "discount_type"} {
		if fd, _, ok := vm.fieldIndex(n); ok {
			wantV[n] = fieldShape(fd)
		}
	}
	if !reflect.DeepEqual(got, wantV) {
		t.Errorf("Voucher fields = %v, want exactly %v", got, wantV)
	}
	cm, _ := f.mustMessage(t, "CreateVoucherRequest")
	gotC := fields(cm)
	for _, n := range []string{"scope", "discount_type"} {
		delete(gotC, n)
	}
	wantC := map[string]string{
		"code": "1 " + str, "discount_value": "4 " + i64, "min_spend": "5 " + i64, "max_discount": "6 " + i64,
		"quota": "7 " + i64, "starts_at": "8 " + tsType, "ends_at": "9 " + tsType,
		"per_user_limit": "10 TYPE_INT32", "new_user_only": "11 TYPE_BOOL",
	}
	if !reflect.DeepEqual(gotC, wantC) {
		t.Errorf("CreateVoucherRequest fields = %v, want exactly %v", gotC, wantC)
	}
	for n, num := range map[string]int{"scope": 2, "discount_type": 3} {
		if fd, _, ok := cm.fieldIndex(n); !ok || fd.Number != num {
			t.Errorf("CreateVoucherRequest.%s lost its number %d", n, num)
		}
	}
	assertMessageFields(t, f, map[string]map[string]string{
		"UpdateVoucherRequest": {
			"voucher_id": "1 " + str, "ends_at": "2 " + tsType, "quota": "3 " + i64,
			"per_user_limit": "4 TYPE_INT32 optional", "new_user_only": "5 TYPE_BOOL optional",
		},
	})
}

// Scenario: Q7 referral welcome contract: ListMyVouchers exists on VoucherService
func TestQ7_ListMyVouchersExistsOnVoucherService(t *testing.T) {
	f := loadFile(t, promotionFile)
	assertRPCs(t, f, "VoucherService", voucherExisting, "ListMyVouchers")
	assertMessageFields(t, f, map[string]map[string]string{
		"ListMyVouchersRequest":  {},
		"ListMyVouchersResponse": {"vouchers": "1 TYPE_MESSAGE .platform.promotion.v1.Voucher repeated"},
	})
}

// Scenario: Q7 referral welcome contract: Voucher with user-bound fields round-trips
func TestQ7_VoucherWithUserBoundFieldsRoundTrips(t *testing.T) {
	f := loadFile(t, promotionFile)
	m, _ := f.mustMessage(t, "Voucher")
	for _, n := range []string{"owner_user_id", "kind", "per_user_limit", "new_user_only"} {
		if _, _, ok := m.fieldIndex(n); !ok {
			t.Fatalf("Voucher.%s missing", n)
		}
	}
	in := map[string]any{"id": "v-1", "code": "WELCOME", "quota": int64(1), "active": true,
		"owner_user_id": "01JABCDEFGHJKMNPQRSTVWXYZ0", "kind": int64(2), "per_user_limit": int64(1), "new_user_only": true}
	got := decode(t, m.Field, encode(m, in))
	for k, v := range in {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	plain := decode(t, m.Field, encode(m, map[string]any{"id": "v-2", "code": "STD", "active": true}))
	if plain["owner_user_id"] != "" || plain["kind"] != int64(0) || plain["per_user_limit"] != int64(0) || plain["new_user_only"] != false {
		t.Errorf("unset user-bound fields parsed as %v", plain)
	}
}

// Scenario: Q7 referral welcome contract: Old readers skip the voucher additions and tolerate an unknown kind
func TestQ7_OldReadersSkipTheVoucherAdditionsAndTolerateAnUnknownKind(t *testing.T) {
	f := loadFile(t, promotionFile)
	m, _ := f.mustMessage(t, "Voucher")
	for _, n := range []string{"owner_user_id", "kind", "per_user_limit", "new_user_only"} {
		if _, _, ok := m.fieldIndex(n); !ok {
			t.Fatalf("Voucher.%s missing", n)
		}
	}
	in := map[string]any{"id": "v-1", "code": "WELCOME", "quota": int64(1), "active": true,
		"owner_user_id": "u-1", "kind": int64(2), "per_user_limit": int64(1), "new_user_only": true}
	got := decode(t, upTo(m.Field, 14), encode(m, in))
	for _, n := range []string{"owner_user_id", "kind", "per_user_limit", "new_user_only"} {
		if _, present := got[n]; present {
			t.Errorf("old schema surfaced new field %s", n)
		}
	}
	if got["id"] != "v-1" || got["code"] != "WELCOME" || got["quota"] != int64(1) || got["active"] != true {
		t.Errorf("fields 1-14 not intact: %v", got)
	}
	unk := decode(t, m.Field, encode(m, map[string]any{"id": "v-3", "kind": int64(99)}))
	if unk["kind"] != int64(99) {
		t.Errorf("unknown kind parsed as %v, want 99", unk["kind"])
	}
}

// Scenario: Q7 referral welcome contract: Update distinguishes unset from zero
func TestQ7_UpdateDistinguishesUnsetFromZero(t *testing.T) {
	f := loadFile(t, promotionFile)
	m, _ := f.mustMessage(t, "UpdateVoucherRequest")
	for _, n := range []string{"per_user_limit", "new_user_only"} {
		fd, _, ok := m.fieldIndex(n)
		if !ok || !fd.Proto3Optional {
			t.Fatalf("UpdateVoucherRequest.%s must exist and be proto3 optional", n)
		}
	}
	zero := encode(m, map[string]any{"voucher_id": "v-1", "per_user_limit": int64(0), "new_user_only": false})
	nums := wireNumbers(t, zero)
	if !reflect.DeepEqual(nums, []int{1, 4, 5}) {
		t.Errorf("explicit zero wire fields = %v, want [1 4 5]", nums)
	}
	got := decode(t, m.Field, zero)
	if got["per_user_limit"] != int64(0) || got["new_user_only"] != false {
		t.Errorf("explicit zero parsed as %v", got)
	}
	unset := encode(m, map[string]any{"voucher_id": "v-1"})
	if nums := wireNumbers(t, unset); !reflect.DeepEqual(nums, []int{1}) {
		t.Errorf("unset wire fields = %v, want [1]", nums)
	}
}

// Scenario: Q7 referral welcome contract: ListMyVouchers authz and scope are documented
func TestQ7_ListMyVouchersAuthzAndScopeAreDocumented(t *testing.T) {
	f := loadFile(t, promotionFile)
	c, ok := f.rpcComment(t, "VoucherService", "ListMyVouchers")
	err := checkKeys(c, ok, "signed-in buyer", "own", "active", "unexpired", "never a request field")
	if err != nil {
		t.Error(err)
	}
}

// Scenario: Q7 referral welcome contract: Server-owned voucher fields are documented
func TestQ7_ServerOwnedVoucherFieldsAreDocumented(t *testing.T) {
	f := loadFile(t, promotionFile)
	for _, fld := range []string{"owner_user_id", "kind"} {
		c, ok := f.fieldComment(t, "Voucher", fld)
		if err := checkKeys(c, ok, "server-owned"); err != nil {
			t.Errorf("Voucher.%s: %v", fld, err)
		}
	}
	c, ok := f.msgComment(t, "CreateVoucherRequest")
	if err := checkKeys(c, ok, "server-owned", "Update"); err != nil {
		t.Errorf("CreateVoucherRequest: %v", err)
	}
}

// Scenario: Q7 referral welcome contract: Voucher refusal reasons are documented
func TestQ7_VoucherRefusalReasonsAreDocumented(t *testing.T) {
	f := loadFile(t, promotionFile)
	c, ok := f.rpcComment(t, "VoucherService", "ValidateAndReserve")
	err := checkKeys(c, ok, "VOUCHER_NOT_ELIGIBLE", "VOUCHER_PER_USER_LIMIT", "valid=false", "UNAVAILABLE",
		"any other buyer", "single use", "completed order")
	if err != nil {
		t.Error(err)
	}
}

// Scenario: Q7 referral welcome contract: Missing server-owned note is caught
func TestQ7_MissingServerOwnedNoteIsCaught(t *testing.T) {
	fixture := "Pseudonymous id of the buyer the voucher is bound to."
	if err := checkKeys(fixture, true, "server-owned"); err == nil {
		t.Fatal("helper accepted a comment lacking server-owned")
	}
	if err := checkKeys("This field is server-owned.", true, "server-owned"); err != nil {
		t.Fatalf("helper rejected a good comment: %v", err)
	}
}

// ---- order ------------------------------------------------------------------

var orderExisting = []string{"CreateOrder", "GetOrder", "ListBuyerOrders", "ListSellerOrders", "UpdateOrderStatus",
	"CancelOrder", "ConfirmReceipt", "CalculateShippingFee", "PreviewCheckout", "GetSagaState", "ForceFailSaga",
	"CreateReturnRequest", "GetReturnRequest", "UpdateReturnStatus", "ListOrderReturns", "CreateShipment",
	"GetShipmentTracking"}

// Scenario: Q7 referral welcome contract: HasCompletedOrder RPC exists
func TestQ7_HasCompletedOrderRPCExists(t *testing.T) {
	f := loadFile(t, orderFile)
	assertRPCs(t, f, "OrderService", orderExisting, "HasCompletedOrder")
	assertMessageFields(t, f, map[string]map[string]string{
		"HasCompletedOrderRequest":  {"buyer_id": "1 " + str},
		"HasCompletedOrderResponse": {"has_completed": "1 TYPE_BOOL"},
	})
}

// Scenario: Q7 referral welcome contract: HasCompletedOrder response round-trips both values
func TestQ7_HasCompletedOrderResponseRoundTripsBothValues(t *testing.T) {
	f := loadFile(t, orderFile)
	m, _ := f.mustMessage(t, "HasCompletedOrderResponse")
	if _, _, ok := m.fieldIndex("has_completed"); !ok {
		t.Fatal("HasCompletedOrderResponse.has_completed missing")
	}
	tb := encode(m, map[string]any{"has_completed": true})
	if got := decode(t, m.Field, tb); got["has_completed"] != true {
		t.Errorf("true parsed as %v", got["has_completed"])
	}
	fb := encode(m, map[string]any{"has_completed": false})
	if len(fb) != 0 {
		t.Errorf("false is on the wire: %v", fb)
	}
	if got := decode(t, m.Field, fb); got["has_completed"] != false {
		t.Errorf("false parsed as %v", got["has_completed"])
	}
}

// Scenario: Q7 referral welcome contract: HasCompletedOrder is documented as internal
func TestQ7_HasCompletedOrderIsDocumentedAsInternal(t *testing.T) {
	f := loadFile(t, orderFile)
	c, ok := f.rpcComment(t, "OrderService", "HasCompletedOrder")
	err := checkKeys(c, ok, "internal", "service-to-service", "team-promotion", "NOT routed at the gateway")
	if err != nil {
		t.Error(err)
	}
}

// Scenario: Q7 referral welcome contract: Missing internal note is caught
func TestQ7_MissingInternalNoteIsCaught(t *testing.T) {
	fixture := "Reports whether the buyer has a completed order. Internal, service-to-service."
	if err := checkKeys(fixture, true, "NOT routed"); err == nil {
		t.Fatal("helper accepted a comment lacking NOT routed")
	}
	if err := checkKeys("Internal. NOT routed at\n the gateway.", true, "NOT routed at the gateway"); err != nil {
		t.Fatalf("helper rejected a wrapped good comment: %v", err)
	}
}
