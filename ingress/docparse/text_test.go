package docparse

import "testing"

func TestSplitFlatText(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		count int
		want  []string
	}{
		{
			name:  "no headings falls back to one chapter",
			text:  "Just some prose.\nMore prose.\n",
			count: 1,
		},
		{
			name:  "two arabic chapters",
			text:  "Front matter.\n\nChapter 1\nIntro text.\n\nChapter 2\nMore text.\n",
			count: 2,
			want:  []string{"Chapter 1", "Chapter 2"},
		},
		{
			name:  "capitulo variant",
			text:  "Capítulo 1: Início\nTexto.\n\nCapítulo 2: Meio\nMais texto.\n",
			count: 2,
			want:  []string{"Capítulo 1: Início", "Capítulo 2: Meio"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chapters := SplitFlatText(tc.text)
			if len(chapters) != tc.count {
				t.Fatalf("got %d chapters, want %d: %+v", len(chapters), tc.count, chapters)
			}
			for i, want := range tc.want {
				if chapters[i].Title != want {
					t.Errorf("chapter %d title = %q, want %q", i, chapters[i].Title, want)
				}
				if chapters[i].Index != i+1 {
					t.Errorf("chapter %d index = %d, want %d", i, chapters[i].Index, i+1)
				}
			}
		})
	}
}

func TestSplitFlatText_NormalizesCRLF(t *testing.T) {
	text := "Chapter 1\r\nIntro.\r\n\r\nChapter 2\r\nMore.\r\n"
	chapters := SplitFlatText(normalizeNewlines(text))
	if len(chapters) != 2 {
		t.Fatalf("got %d chapters, want 2: %+v", len(chapters), chapters)
	}
	for _, ch := range chapters {
		if containsCR(ch.Text) {
			t.Errorf("chapter %d text still contains CR: %q", ch.Index, ch.Text)
		}
	}
}

func containsCR(s string) bool {
	for _, r := range s {
		if r == '\r' {
			return true
		}
	}
	return false
}
