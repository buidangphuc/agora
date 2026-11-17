package aiv1_test

// Contract tests for change docs-ai-data-contract (capability ai-data-contract): AI response provenance fields.

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

var aiProvenance = map[string][2]int32{
	"ShoppingAssistantResponse": {4, 5},
	"MagicListingResponse":      {7, 8},
	"ChatCopilotResponse":       {2, 3},
	"SummarizeReviewsResponse":  {5, 6},
}

var aiBaseline = map[string]map[string]string{
	"ShoppingAssistantResponse": {"reply_text": "1 string", "product_cards": "2 repeated message:platform.ai.v1.ProductCardSnippet", "suggested_followups": "3 repeated string"},
	"MagicListingResponse": {"generated_title": "1 string", "generated_description": "2 string", "suggested_category_id": "3 string",
		"suggested_price_min": "4 int64", "suggested_price_max": "5 int64", "highlight_tags": "6 repeated string"},
	"ChatCopilotResponse":      {"quick_replies": "1 repeated string"},
	"SummarizeReviewsResponse": {"summary": "1 string", "pros": "2 repeated string", "cons": "3 repeated string", "sentiment": "4 string"},
	"ProductCardSnippet": {"listing_id": "1 string", "title": "2 string", "price": "3 int64", "currency": "4 string",
		"image_url": "5 string", "discount_rate": "6 int32", "rating_text": "7 string"},
	"ShoppingAssistantRequest": {"user_id": "1 string", "message": "2 string", "previous_context": "3 repeated string"},
	"MagicListingRequest":      {"title_hint": "1 string", "category_hint": "2 string", "image_url": "3 string"},
	"ChatCopilotRequest":       {"seller_id": "1 string", "buyer_message": "2 string", "listing_id": "3 string"},
	"ReviewInput":              {"rating": "1 int32", "comment": "2 string"},
	"SummarizeReviewsRequest":  {"listing_id": "1 string", "reviews": "2 repeated message:platform.ai.v1.ReviewInput"},
	"ClassifyTagsResponse":     {"tags": "1 repeated message:platform.ai.v1.ClassifiedTag", "skus": "2 repeated message:platform.ai.v1.SkuClassification"},
	"ScreenListingResponse":    {"verdict": "1 enum:platform.ai.v1.ScreenVerdict", "labels": "2 repeated string", "backend": "3 string"},
}

// Scenario: Provenance fields present at the specified numbers
func TestAiData_ProvenanceFieldsShape(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	shapes := map[string]map[string]string{}
	for msg, nums := range aiProvenance {
		want := map[string]string{}
		for k, v := range aiBaseline[msg] {
			want[k] = v
		}
		want["request_id"] = itoa(nums[0]) + " string"
		want["model_version"] = itoa(nums[1]) + " string"
		shapes[msg] = want
	}
	for msg, want := range shapes {
		tcShape(t, fd, msg, want)
	}
}

func itoa(n int32) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return "?"
}

// Scenario: Existing AI surface is untouched
func TestAiData_ExistingAiSurfaceUntouched(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	tcRPCs(t, fd, "AIService", map[string][2]string{
		"ShoppingAssistant": {"ShoppingAssistantRequest", "ShoppingAssistantResponse"},
		"MagicListing":      {"MagicListingRequest", "MagicListingResponse"},
		"ChatCopilot":       {"ChatCopilotRequest", "ChatCopilotResponse"},
		"SummarizeReviews":  {"SummarizeReviewsRequest", "SummarizeReviewsResponse"},
		"ClassifyTags":      {"ClassifyTagsRequest", "ClassifyTagsResponse"},
		"ScreenListing":     {"ScreenListingRequest", "ScreenListingResponse"},
	})
	for msg, fields := range aiBaseline {
		md := fd.Messages().ByName(protoreflect.Name(msg))
		if md == nil {
			t.Errorf("message %s missing", msg)
			continue
		}
		for name, want := range fields {
			f := md.Fields().ByName(protoreflect.Name(name))
			if f == nil {
				t.Errorf("%s.%s missing", msg, name)
				continue
			}
			got := itoa(int32(f.Number())) + " " + tcKind(f)
			if got != want {
				t.Errorf("%s.%s = %q, want %q", msg, name, got, want)
			}
		}
	}
}

// Scenario: Provenance format is documented
func TestAiData_ProvenanceDocumented(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	for msg := range aiProvenance {
		tcCheck(t, msg+".request_id", tcFieldComment(t, fd, msg, "request_id"), "UUID", "hex", "per call")
		tcCheck(t, msg+".model_version", tcFieldComment(t, fd, msg, "model_version"), "<backend>:<prompt_version>", "rules-v1")
	}
	tcCheck(t, "ShoppingAssistantResponse.model_version", tcFieldComment(t, fd, "ShoppingAssistantResponse", "model_version"),
		"|<search model_version>")
}

// Scenario: Old client reads a new response
func TestAiData_WireCompatible(t *testing.T) {
	fd, files := tcFile(t, aiProto)
	newMD := tcMsg(t, fd, "ShoppingAssistantResponse")
	old := tcOld(t, files, fd, func(p *descriptorpb.FileDescriptorProto) {
		tcDropFields(p, "ShoppingAssistantResponse", func(n int32) bool { return n <= 3 })
	})
	oldMD := tcMsg(t, old, "ShoppingAssistantResponse")

	n := tcNew(t, newMD, "reply_text", "hi", "suggested_followups", []string{"a", "b"},
		"request_id", "0123456789abcdef0123456789abcdef", "model_version", "mock:v1|search-v2")
	got := tcParseWith(t, n, oldMD)
	if got.Get(tcFld(t, oldMD, "reply_text")).String() != "hi" ||
		got.Get(tcFld(t, oldMD, "suggested_followups")).List().Len() != 2 {
		t.Errorf("old reader lost existing fields")
	}
	if got.Get(tcFld(t, oldMD, "product_cards")).List().Len() != 0 {
		t.Errorf("old reader invented product_cards")
	}
	if len(got.GetUnknown()) == 0 {
		t.Errorf("old reader should hold request_id/model_version as unknown fields")
	}

	oldResp := tcNew(t, oldMD, "reply_text", "hi")
	back := tcParseWith(t, oldResp, newMD)
	if back.Get(tcFld(t, newMD, "request_id")).String() != "" || back.Get(tcFld(t, newMD, "model_version")).String() != "" {
		t.Errorf("new reader must see empty provenance on an old response")
	}
}

// Scenario: Unset provenance is an empty string, never a placeholder
func TestAiData_UnsetProvenanceIsEmpty(t *testing.T) {
	fd, _ := tcFile(t, aiProto)
	for msg := range aiProvenance {
		md := tcMsg(t, fd, msg)
		m := tcNew(t, md)
		for _, f := range []string{"request_id", "model_version"} {
			if got := m.Get(tcFld(t, md, f)).String(); got != "" {
				t.Errorf("%s.%s = %q, want empty", msg, f, got)
			}
		}
		b, err := proto.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) != 0 {
			t.Errorf("%s: unset provenance added %d bytes", msg, len(b))
		}
	}
}
