package diagstore

import (
	"errors"
	"fmt"
	"strings"

	"wit_component/ohrats_rc_diagnostics_types"
)

func validateReport(value ohrats_rc_diagnostics_types.Report) error {
	if err := validateToken(value.Source, 96, "diagnostic source"); err != nil {
		return err
	}
	if err := validateToken(value.Code, 96, "diagnostic code"); err != nil {
		return err
	}
	if value.Message == "" || len(value.Message) > 512 {
		return errors.New("diagnostic message must contain 1 to 512 bytes")
	}
	if len(value.Fields) > maxFields {
		return errors.New("diagnostic report has too many fields")
	}
	for _, field := range value.Fields {
		if err := validateField(field); err != nil {
			return err
		}
	}
	return nil
}

func validateField(field ohrats_rc_diagnostics_types.Field) error {
	if err := validateToken(field.Name, 64, "diagnostic field"); err != nil {
		return err
	}
	if len(field.Value) > 256 {
		return errors.New("diagnostic field value exceeds 256 bytes")
	}
	name := strings.ToLower(field.Name)
	for _, word := range []string{
		"command",
		"credential",
		"input",
		"key",
		"output",
		"secret",
		"token",
		"transcript",
	} {
		if strings.Contains(name, word) {
			return fmt.Errorf("diagnostic field %q may contain plaintext", field.Name)
		}
	}
	return nil
}

func validateToken(value string, maximum int, label string) error {
	if value == "" || len(value) > maximum {
		return fmt.Errorf("invalid %s %q", label, value)
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		ascii := character >= '0' && character <= '9' ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z'
		if ascii {
			continue
		}
		switch character {
		case '.', '-', '_', ':', '/':
		default:
			return fmt.Errorf("invalid %s %q", label, value)
		}
	}
	return nil
}
