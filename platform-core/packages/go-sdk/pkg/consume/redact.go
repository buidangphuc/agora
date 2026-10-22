package consume

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
)

func orRandomFingerprinter(f *kafkax.Fingerprinter) *kafkax.Fingerprinter {
	if f != nil {
		return f
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("consume: no randomness: " + err.Error())
	}
	f, _ = kafkax.NewFingerprinter(hex.EncodeToString(b))
	return f
}

type keyScrubber struct {
	fp *kafkax.Fingerprinter
}

func (s keyScrubber) fingerprints(recs ...*kgo.Record) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, r := range recs {
		fp := s.fp.Of(r.Key)
		if _, ok := seen[fp]; !ok {
			seen[fp] = struct{}{}
			out = append(out, fp)
		}
	}
	return out
}

func (s keyScrubber) scrub(msg string, recs ...*kgo.Record) string {
	keys := make([]string, 0, len(recs))
	for _, r := range recs {
		if len(r.Key) > 0 {
			keys = append(keys, string(r.Key))
		}
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		msg = strings.ReplaceAll(msg, k, "[key_fp:"+s.fp.Of([]byte(k))+"]")
	}
	return msg
}

type scrubbedError string

func (e scrubbedError) Error() string { return string(e) }

type scrubbedWrap struct {
	msg   string
	cause error
}

func (e scrubbedWrap) Error() string { return e.msg }
func (e scrubbedWrap) Unwrap() error { return e.cause }
