package validation

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		card    string
		wantErr error
	}{
		{name: "valid", card: "4532015112830366"},
		{name: "empty", card: "", wantErr: ErrEmptyCard},
		{name: "non numeric", card: "453201511283036a", wantErr: ErrNonNumeric},
		{name: "invalid length", card: "453201511283036", wantErr: ErrInvalidLength},
		{name: "invalid checksum", card: "4532015112830367", wantErr: ErrInvalidChecksum},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.card)
			if test.wantErr == nil && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
			if test.wantErr != nil && err != test.wantErr {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestMask(t *testing.T) {
	got := Mask("4532015112830366")
	want := "4532********0366"
	if got != want {
		t.Fatalf("Mask() = %q, want %q", got, want)
	}
	if Mask("not-a-card") != "********" {
		t.Fatal("Mask() must hide non-card input")
	}
}
