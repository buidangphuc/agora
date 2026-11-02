package promotionv1_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

type gcField struct {
	Name           string `json:"name"`
	Number         int    `json:"number"`
	Type           string `json:"type"`
	TypeName       string `json:"typeName"`
	Label          string `json:"label"`
	Proto3Optional bool   `json:"proto3Optional"`
}

type gcMsg struct {
	Name  string    `json:"name"`
	Field []gcField `json:"field"`
}

type gcEnumValue struct {
	Name   string `json:"name"`
	Number int    `json:"number"`
}

type gcEnum struct {
	Name  string        `json:"name"`
	Value []gcEnumValue `json:"value"`
}

type gcMethod struct {
	Name       string `json:"name"`
	InputType  string `json:"inputType"`
	OutputType string `json:"outputType"`
}

type gcService struct {
	Name   string     `json:"name"`
	Method []gcMethod `json:"method"`
}

type gcFile struct {
	Name        string      `json:"name"`
	MessageType []gcMsg     `json:"messageType"`
	EnumType    []gcEnum    `json:"enumType"`
	Service     []gcService `json:"service"`
}

const gcPromoFile = "platform/promotion/v1/promotion.proto"
const gcPkg = ".platform.promotion.v1."
const gcTimestamp = ".google.protobuf.Timestamp"
const gcPageReq = ".platform.common.v1.PageRequest"
const gcPageResp = ".platform.common.v1.PageResponse"

func gcLoad(t *testing.T) gcFile {
	t.Helper()
	cmd := exec.Command("buf", "build", "--path", gcPromoFile, "-o", "-#format=json")
	cmd.Dir = "../../.."
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("buf build failed (harness): %v\n%s", err, errb.String())
	}
	var set struct {
		File []gcFile `json:"file"`
	}
	if err := json.Unmarshal(out.Bytes(), &set); err != nil {
		t.Fatalf("decode descriptor json: %v", err)
	}
	for _, f := range set.File {
		if f.Name == gcPromoFile {
			return f
		}
	}
	t.Fatalf("%s not in descriptor set", gcPromoFile)
	return gcFile{}
}

func (f gcFile) msg(t *testing.T, name string) (gcMsg, bool) {
	t.Helper()
	for _, m := range f.MessageType {
		if m.Name == name {
			return m, true
		}
	}
	t.Errorf("message %s%s missing from descriptor", gcPkg, name)
	return gcMsg{}, false
}

func (f gcFile) rpcs(t *testing.T, svc string) map[string][2]string {
	t.Helper()
	got := map[string][2]string{}
	found := false
	for _, s := range f.Service {
		if s.Name != svc {
			continue
		}
		found = true
		for _, m := range s.Method {
			got[m.Name] = [2]string{strings.TrimPrefix(m.InputType, gcPkg), strings.TrimPrefix(m.OutputType, gcPkg)}
		}
	}
	if !found {
		t.Fatalf("service %s missing", svc)
	}
	return got
}

func (m gcMsg) field(name string) (gcField, bool) {
	for _, fd := range m.Field {
		if fd.Name == name {
			return fd, true
		}
	}
	return gcField{}, false
}

type wantField struct {
	num      int
	typ      string
	typeName string
	repeated bool
	optional bool
}

func gcCheckFields(t *testing.T, f gcFile, msgName string, want map[string]wantField) {
	t.Helper()
	m, ok := f.msg(t, msgName)
	if !ok {
		return
	}
	for name, w := range want {
		fd, ok := m.field(name)
		if !ok {
			t.Errorf("field %s.%s missing", msgName, name)
			continue
		}
		if fd.Number != w.num {
			t.Errorf("%s.%s number = %d, want %d", msgName, name, fd.Number, w.num)
		}
		if fd.Type != w.typ {
			t.Errorf("%s.%s type = %s, want %s", msgName, name, fd.Type, w.typ)
		}
		if w.typeName != "" && fd.TypeName != w.typeName {
			t.Errorf("%s.%s typeName = %s, want %s", msgName, name, fd.TypeName, w.typeName)
		}
		if w.repeated != (fd.Label == "LABEL_REPEATED") {
			t.Errorf("%s.%s repeated = %v, want %v", msgName, name, fd.Label == "LABEL_REPEATED", w.repeated)
		}
		if w.optional != fd.Proto3Optional {
			t.Errorf("%s.%s proto3 optional = %v, want %v", msgName, name, fd.Proto3Optional, w.optional)
		}
	}
}

func gcCheckRPCs(t *testing.T, f gcFile, svc string, want map[string][2]string) {
	t.Helper()
	got := f.rpcs(t, svc)
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("rpc %s.%s missing", svc, name)
			continue
		}
		if g != w {
			t.Errorf("rpc %s.%s = %v, want %v", svc, name, g, w)
		}
	}
}

// Scenario: Promotion admin contract is additive: Voucher RPCs and fields exist
func TestGapsContract_VoucherRPCsAndFields(t *testing.T) {
	f := gcLoad(t)
	gcCheckRPCs(t, f, "VoucherService", map[string][2]string{
		"DeactivateVoucher": {"DeactivateVoucherRequest", "DeactivateVoucherResponse"},
		"UpdateVoucher":     {"UpdateVoucherRequest", "UpdateVoucherResponse"},
	})
	gcCheckFields(t, f, "Voucher", map[string]wantField{
		"active":         {num: 13, typ: "TYPE_BOOL"},
		"deactivated_at": {num: 14, typ: "TYPE_MESSAGE", typeName: gcTimestamp},
	})
	gcCheckFields(t, f, "DeactivateVoucherRequest", map[string]wantField{
		"voucher_id": {num: 1, typ: "TYPE_STRING"},
	})
	gcCheckFields(t, f, "DeactivateVoucherResponse", map[string]wantField{
		"voucher": {num: 1, typ: "TYPE_MESSAGE", typeName: gcPkg + "Voucher"},
	})
	gcCheckFields(t, f, "UpdateVoucherRequest", map[string]wantField{
		"voucher_id": {num: 1, typ: "TYPE_STRING"},
		"ends_at":    {num: 2, typ: "TYPE_MESSAGE", typeName: gcTimestamp, optional: true},
		"quota":      {num: 3, typ: "TYPE_INT64", optional: true},
	})
	gcCheckFields(t, f, "UpdateVoucherResponse", map[string]wantField{
		"voucher": {num: 1, typ: "TYPE_MESSAGE", typeName: gcPkg + "Voucher"},
	})
}

// Scenario: Promotion admin contract is additive: Campaign RPCs, enum and fields exist
func TestGapsContract_CampaignRPCsEnumAndFields(t *testing.T) {
	f := gcLoad(t)
	gcCheckRPCs(t, f, "FlashSaleService", map[string][2]string{
		"ListCampaigns":  {"ListCampaignsRequest", "ListCampaignsResponse"},
		"CancelCampaign": {"CancelCampaignRequest", "CancelCampaignResponse"},
		"UpdateCampaign": {"UpdateCampaignRequest", "UpdateCampaignResponse"},
	})

	var enum *gcEnum
	for i := range f.EnumType {
		if f.EnumType[i].Name == "CampaignStatus" {
			enum = &f.EnumType[i]
		}
	}
	if enum == nil {
		t.Errorf("enum %sCampaignStatus missing", gcPkg)
	} else {
		var got []string
		for _, v := range enum.Value {
			got = append(got, fmt.Sprintf("%s=%d", v.Name, v.Number))
		}
		sort.Strings(got)
		want := []string{
			"CAMPAIGN_STATUS_ACTIVE=2", "CAMPAIGN_STATUS_CANCELLED=4", "CAMPAIGN_STATUS_ENDED=3",
			"CAMPAIGN_STATUS_SCHEDULED=1", "CAMPAIGN_STATUS_UNSPECIFIED=0",
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("CampaignStatus values = %v, want exactly %v", got, want)
		}
	}

	gcCheckFields(t, f, "FlashSaleCampaign", map[string]wantField{
		"status":       {num: 9, typ: "TYPE_ENUM", typeName: gcPkg + "CampaignStatus"},
		"cancelled_at": {num: 10, typ: "TYPE_MESSAGE", typeName: gcTimestamp},
	})
	gcCheckFields(t, f, "CreateCampaignRequest", map[string]wantField{
		"idempotency_key": {num: 7, typ: "TYPE_STRING"},
	})
	gcCheckFields(t, f, "ListCampaignsResponse", map[string]wantField{
		"flash_sale_disabled": {num: 3, typ: "TYPE_BOOL"},
	})
	gcCheckFields(t, f, "ListActiveCampaignsResponse", map[string]wantField{
		"flash_sale_disabled": {num: 3, typ: "TYPE_BOOL"},
	})
	gcCheckFields(t, f, "CancelCampaignRequest", map[string]wantField{
		"campaign_id": {num: 1, typ: "TYPE_STRING"},
	})
	gcCheckFields(t, f, "CancelCampaignResponse", map[string]wantField{
		"campaign": {num: 1, typ: "TYPE_MESSAGE", typeName: gcPkg + "FlashSaleCampaign"},
	})
	gcCheckFields(t, f, "UpdateCampaignRequest", map[string]wantField{
		"campaign_id": {num: 1, typ: "TYPE_STRING"},
		"sale_price":  {num: 2, typ: "TYPE_INT64"},
		"stock_cap":   {num: 3, typ: "TYPE_INT64"},
		"starts_at":   {num: 4, typ: "TYPE_MESSAGE", typeName: gcTimestamp},
		"ends_at":     {num: 5, typ: "TYPE_MESSAGE", typeName: gcTimestamp},
	})
	gcCheckFields(t, f, "UpdateCampaignResponse", map[string]wantField{
		"campaign": {num: 1, typ: "TYPE_MESSAGE", typeName: gcPkg + "FlashSaleCampaign"},
	})
}

// Scenario: Promotion admin contract is additive: Pagination reuses the common message
func TestGapsContract_ListCampaignsShape(t *testing.T) {
	f := gcLoad(t)
	gcCheckFields(t, f, "ListCampaignsRequest", map[string]wantField{
		"statuses": {num: 1, typ: "TYPE_ENUM", typeName: gcPkg + "CampaignStatus", repeated: true},
		"page":     {num: 2, typ: "TYPE_MESSAGE", typeName: gcPageReq},
	})
	gcCheckFields(t, f, "ListCampaignsResponse", map[string]wantField{
		"campaigns": {num: 1, typ: "TYPE_MESSAGE", typeName: gcPkg + "FlashSaleCampaign", repeated: true},
		"page":      {num: 2, typ: "TYPE_MESSAGE", typeName: gcPageResp},
	})
	if m, ok := f.msg(t, "ListCampaignsRequest"); ok && len(m.Field) != 2 {
		t.Errorf("ListCampaignsRequest has %d fields, want exactly 2 (statuses, page)", len(m.Field))
	}
}

func gcToDesc(m gcMsg) msgDesc {
	var fs []fieldDesc
	for _, fd := range m.Field {
		fs = append(fs, fieldDesc{Name: fd.Name, Number: fd.Number, Type: fd.Type, TypeName: fd.TypeName, Label: fd.Label})
	}
	return msgDesc{Name: m.Name, Field: fs}
}

func gcOldSchema(name string, fs ...fieldDesc) msgDesc {
	return msgDesc{Name: name, Field: fs}
}

// Scenario: Promotion admin contract is additive: Old payload decodes under the new schema
func TestGapsContract_OldPayloadDecodes(t *testing.T) {
	f := gcLoad(t)
	s := func(n string, num int) fieldDesc { return fieldDesc{Name: n, Number: num, Type: "TYPE_STRING"} }
	i := func(n string, num int) fieldDesc { return fieldDesc{Name: n, Number: num, Type: "TYPE_INT64"} }
	e := func(n string, num int) fieldDesc { return fieldDesc{Name: n, Number: num, Type: "TYPE_ENUM"} }

	cases := []struct {
		msg     string
		old     msgDesc
		vals    map[string]any
		wantNew map[string]any
	}{
		{
			msg: "Voucher",
			old: gcOldSchema("Voucher", s("id", 1), s("code", 2), e("scope", 3), s("seller_id", 4), e("discount_type", 5),
				i("discount_value", 6), i("min_spend", 7), i("max_discount", 8), i("quota", 9), i("used", 10)),
			vals: map[string]any{"id": "v1", "code": "SAVE10", "scope": int64(2), "seller_id": "s1", "discount_type": int64(1),
				"discount_value": int64(10), "min_spend": int64(100), "max_discount": int64(50), "quota": int64(5), "used": int64(2)},
			wantNew: map[string]any{"active": false},
		},
		{
			msg: "FlashSaleCampaign",
			old: gcOldSchema("FlashSaleCampaign", s("id", 1), s("listing_id", 2), s("variant_id", 3), i("sale_price", 4),
				i("stock_cap", 5), i("stock_sold", 6)),
			vals: map[string]any{"id": "c1", "listing_id": "l1", "variant_id": "x1", "sale_price": int64(900),
				"stock_cap": int64(10), "stock_sold": int64(3)},
			wantNew: map[string]any{"status": int64(0)},
		},
		{
			msg:     "CreateCampaignRequest",
			old:     gcOldSchema("CreateCampaignRequest", s("listing_id", 1), s("variant_id", 2), i("sale_price", 3), i("stock_cap", 4)),
			vals:    map[string]any{"listing_id": "l1", "variant_id": "x1", "sale_price": int64(900), "stock_cap": int64(10)},
			wantNew: map[string]any{"idempotency_key": ""},
		},
	}
	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			m, ok := f.msg(t, c.msg)
			if !ok {
				return
			}
			nd := gcToDesc(m)
			payload := encode(c.old, c.vals)
			if len(payload) == 0 {
				t.Fatal("harness: empty old payload")
			}
			got := decode(t, nd.Field, payload)
			for k, v := range c.vals {
				if got[k] != v {
					t.Errorf("%s.%s = %v, want %v (pre-existing field lost)", c.msg, k, got[k], v)
				}
			}
			for k, v := range c.wantNew {
				g, present := got[k]
				if !present {
					t.Errorf("new field %s.%s missing from schema", c.msg, k)
					continue
				}
				if g != v {
					t.Errorf("%s.%s = %v, want zero value %v", c.msg, k, g, v)
				}
			}
		})
	}
}
