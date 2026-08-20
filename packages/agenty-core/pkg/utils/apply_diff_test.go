package utils

import (
	"strings"
	"testing"
)

func TestApplyDiff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		diff  string
		mode  ApplyDiffMode
		want  string
	}{
		{
			name: "create file with blank line",
			diff: "+hello\n+world\n+",
			mode: ApplyDiffCreate,
			want: "hello\nworld\n",
		},
		{
			name: "create empty file",
			mode: ApplyDiffCreate,
			want: "",
		},
		{
			name: "create file normalizes CRLF",
			diff: "+hello\r\n+\r\n+world\r\n",
			mode: ApplyDiffCreate,
			want: "hello\n\nworld",
		},
		{
			name:  "empty diff preserves existing file",
			input: "one\ntwo\n",
			want:  "one\ntwo\n",
		},
		{
			name:  "floating insertion into empty file",
			input: "",
			diff:  "@@\n+hello\n+world",
			want:  "hello\nworld\n",
		},
		{
			name:  "floating hunk",
			input: "- Milk\n- Bread\n- Eggs\n- Apples\n- Coffee",
			diff:  "@@\n - Milk\n - Bread\n - Eggs\n-- Apples\n-- Coffee\n+- [x] Apples\n+- [x] Coffee",
			want:  "- Milk\n- Bread\n- Eggs\n- [x] Apples\n- [x] Coffee",
		},
		{
			name:  "anchored replacement preserves trailing newline",
			input: "line1\nline2\nline3\n",
			diff:  "@@ line1\n-line2\n+updated\n line3",
			want:  "line1\nupdated\nline3\n",
		},
		{
			name:  "deletion with context",
			input: "keep\nremove me\nstay\n",
			diff:  "@@ keep\n-remove me\n stay",
			want:  "keep\nstay\n",
		},
		{
			name:  "pure insertion with blank context lines",
			input: "import os\n\ndef main():\n    return 1\n",
			diff:  " import os\n+import sys\n\n def main():\n     return 1",
			want:  "import os\nimport sys\n\ndef main():\n    return 1\n",
		},
		{
			name:  "multiple anchored sections",
			input: "class Foo:\n    def baz(self):\n        return 1\n\ndef main():\n    print(Foo().baz())\n",
			diff:  "@@ class Foo:\n-    def baz(self):\n+    def value(self):\n         return 1\n@@ def main():\n-    print(Foo().baz())\n+    print(Foo().value())",
			want:  "class Foo:\n    def value(self):\n        return 1\n\ndef main():\n    print(Foo().value())\n",
		},
		{
			name:  "stacked anchors",
			input: "class First\n    def target():\n        pass\n\nclass Second\n    def target():\n        pass\n",
			diff:  "@@ class Second\n@@     def target():\n-        pass\n+        return 1",
			want:  "class First\n    def target():\n        pass\n\nclass Second\n    def target():\n        return 1\n",
		},
		{
			name:  "reuses parent anchor in a later hunk",
			input: "class Target\n    def first():\n        pass\n\n    def second():\n        pass\n",
			diff:  "@@ class Target\n@@     def first():\n-        pass\n+        return 1\n@@ class Target\n@@     def second():\n-        pass\n+        return 2",
			want:  "class Target\n    def first():\n        return 1\n\n    def second():\n        return 2\n",
		},
		{
			name:  "reuses trimmed parent anchor in a later hunk",
			input: "  class Target  \n    def first():\n        pass\n\n    def second():\n        pass\n",
			diff:  "@@ class Target\n@@     def first():\n-        pass\n+        return 1\n@@ class Target\n@@     def second():\n-        pass\n+        return 2",
			want:  "  class Target  \n    def first():\n        return 1\n\n    def second():\n        return 2\n",
		},
		{
			name:  "single missing anchor remains best effort",
			input: "one\ntwo\n",
			diff:  "@@ missing\n-one\n+first",
			want:  "first\ntwo\n",
		},
		{
			name:  "trailing bare anchor",
			input: "class Only\n    def run():\n        pass\n",
			diff:  "@@ class Only\n@@\n-        pass\n+        return 1",
			want:  "class Only\n    def run():\n        return 1\n",
		},
		{
			name:  "end of file",
			input: "Line A\nLine B\nLine C",
			diff:  "@@\n Line B\n-Line C\n+Line C updated\n*** End of File",
			want:  "Line A\nLine B\nLine C updated",
		},
		{
			name:  "trailing whitespace fuzz",
			input: "one   \ntwo\n",
			diff:  " one\n-two\n+second",
			want:  "one   \nsecond\n",
		},
		{
			name:  "leading and trailing whitespace fuzz",
			input: "  target  \nnext\n",
			diff:  " target\n-next\n+done",
			want:  "  target  \ndone\n",
		},
		{
			name:  "traditional line marker is a best effort anchor",
			input: "one\ntwo\n",
			diff:  "@@ -1,2 +1,2 @@\n one\n-two\n+2",
			want:  "one\n2\n",
		},
		{
			name:  "update diff normalizes CRLF",
			input: "one\ntwo\n",
			diff:  " one\r\n-two\r\n+second\r\n",
			want:  "one\nsecond\n",
		},
		{
			name:  "end of file marker falls back to earlier context",
			input: "target\nmiddle\nend",
			diff:  " target\n+after\n*** End of File",
			want:  "target\nafter\nmiddle\nend",
		},
		{
			name:  "context-only diff leaves content unchanged",
			input: "legacy content",
			diff:  " legacy content",
			want:  "legacy content",
		},
		{
			name:  "replacement works without hunk marker",
			input: "before\nkeep",
			diff:  "-before\n+after",
			want:  "after\nkeep",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ApplyDiff(test.input, test.diff, test.mode)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("ApplyDiff() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplyDiffRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		diff    string
		mode    ApplyDiffMode
		wantErr string
	}{
		{
			name:    "unsupported mode",
			mode:    ApplyDiffMode(255),
			wantErr: "unsupported mode 255",
		},
		{
			name:    "create line without plus",
			diff:    "+valid\ninvalid",
			mode:    ApplyDiffCreate,
			wantErr: "invalid add file line",
		},
		{
			name:    "missing context",
			input:   "one\ntwo\n",
			diff:    " missing\n-two\n+second",
			wantErr: "invalid context",
		},
		{
			name:    "missing first stacked anchor",
			input:   "class Wrong\n    def desired():\n        pass\n",
			diff:    "@@ class Target\n@@     def desired():\n-        pass\n+        return 1",
			wantErr: "invalid anchor",
		},
		{
			name:    "missing second stacked anchor",
			input:   "class Target\n    def desired():\n        pass\n",
			diff:    "@@ class Target\n@@     def missing():\n-        pass\n+        return 1",
			wantErr: "invalid anchor",
		},
		{
			name:    "missing anchor followed by bare marker",
			input:   "one\ntwo\n",
			diff:    "@@ missing\n@@\n-two\n+second",
			wantErr: "invalid anchor",
		},
		{
			name:    "invalid unprefixed update line",
			input:   "one\n",
			diff:    "one",
			wantErr: "invalid line",
		},
		{
			name:    "unknown patch directive",
			input:   "one\n",
			diff:    "*** Unknown Directive",
			wantErr: "invalid line",
		},
		{
			name:    "empty section",
			input:   "one\n",
			diff:    "@@",
			wantErr: "nothing in section",
		},
		{
			name:    "invalid EOF context",
			input:   "one\ntwo",
			diff:    " missing\n*** End of File",
			wantErr: "invalid EOF context",
		},
		{
			name:    "content after EOF marker needs another anchor",
			input:   "one",
			diff:    " one\n*** End of File\n two",
			wantErr: "invalid line",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := ApplyDiff(test.input, test.diff, test.mode)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ApplyDiff() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}
