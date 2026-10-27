package gateway

import "testing"

// batchedStartsFrames are a Messages turn that gives every content_block_start
// before any block's input, the way some relays do: three tool_use blocks, the
// empty text blocks they put between them, then each block's input and stop.
var batchedStartsFrames = []string{
	`{"type":"message_start","message":{"id":"msg_1","model":"m","usage":{"input_tokens":5,"output_tokens":1}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_stop","index":1}`,
	`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_a","name":"read_file","input":{}}}`,
	`{"type":"content_block_start","index":3,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_stop","index":3}`,
	`{"type":"content_block_start","index":4,"content_block":{"type":"tool_use","id":"call_b","name":"read_file","input":{}}}`,
	`{"type":"content_block_start","index":5,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_stop","index":5}`,
	`{"type":"content_block_start","index":6,"content_block":{"type":"tool_use","id":"call_c","name":"read_file","input":{}}}`,
	`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"a.py\"}"}}`,
	`{"type":"content_block_stop","index":2}`,
	`{"type":"content_block_delta","index":4,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"b.py\"}"}}`,
	`{"type":"content_block_stop","index":4}`,
	`{"type":"content_block_delta","index":6,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"c.py\"}"}}`,
	`{"type":"content_block_stop","index":6}`,
	`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
}

type emitted struct {
	kind string
	id   string
	text string
}

func collectToolEvents(t *testing.T, frames []string) []emitted {
	t.Helper()
	d := &anthropicDecoder{server: map[int]bool{}, whole: map[int]bool{}, held: map[int]string{}, open: -1}
	var got []emitted
	emit := func(ev Event) {
		switch ev.Kind {
		case KToolStart:
			got = append(got, emitted{"start", ev.ID, ""})
		case KToolArgs:
			got = append(got, emitted{"args", "", ev.Text})
		}
	}
	for i, f := range frames {
		if err := d.decode(f, emit); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	return got
}

// A start given before the call before it has its input would close that
// call's item with no arguments: the start waits for its own block, and
// each call goes to the client start-then-arguments, in order.
func TestToolStartsHeldWhenUpstreamBatchesThem(t *testing.T) {
	got := collectToolEvents(t, batchedStartsFrames)
	want := []emitted{
		{"start", "call_a", ""}, {"args", "", `{"path": "a.py"}`},
		{"start", "call_b", ""}, {"args", "", `{"path": "b.py"}`},
		{"start", "call_c", ""}, {"args", "", `{"path": "c.py"}`},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %v, want %v\nall: %v", i, got[i], want[i], got)
		}
	}
}

// Anthropic's own order, a block at a time, passes through as it came.
func TestToolStartsPassThroughWhenInterleaved(t *testing.T) {
	frames := []string{
		`{"type":"message_start","message":{"id":"msg_1","model":"m","usage":{"input_tokens":5,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_a","name":"read_file","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"a.py\"}"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_b","name":"read_file","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\": \"b.py\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
	}
	got := collectToolEvents(t, frames)
	want := []emitted{
		{"start", "call_a", ""}, {"args", "", `{"path": "a.py"}`},
		{"start", "call_b", ""}, {"args", "", `{"path": "b.py"}`},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %v, want %v\nall: %v", i, got[i], want[i], got)
		}
	}
}
