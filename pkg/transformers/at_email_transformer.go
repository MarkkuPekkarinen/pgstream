// SPDX-License-Identifier: Apache-2.0

package transformers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
)

var bytesOutputAlphabetLength = byte(len(bytesOutputAlphabet))
var bytesKeep = []byte("',\\{}")
var bytesOutputAlphabet = []byte("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// Modifies `s` in-place.
func scrambleBytes(s []byte, salt string) []byte {
	isArray := len(s) >= 2 && s[0] == '{' && s[len(s)-1] == '}'

	hash := sha256.New()
	// Hard-coding this constant wins less than 3% in BenchmarkScrambleBytes
	const sumLength = 32 // SHA256/8
	hash.Write([]byte(salt))
	hash.Write(s)
	sumBytes := hash.Sum(nil)

	reader := bytes.NewReader(s)
	var r rune
	var err error
	for i := 0; ; i++ {
		r, _, err = reader.ReadRune()
		if err != nil {
			s = s[:i]
			break
		}
		if !isArray || bytes.IndexRune(bytesKeep, r) == -1 {
			// Do not insert, so should not obstruct reader.
			s[i] = bytesOutputAlphabet[(sumBytes[i%sumLength]+byte(r))%bytesOutputAlphabetLength]
		} else {
			// Possibly shift bytes to beginning of s.
			s[i] = byte(r)
		}
	}
	return s
}

func scrambleOneEmail(s []byte, excludeDomain string, replacementDomain string, salt string) []byte {
	atIndex := bytes.IndexRune(s, '@')
	mailbox := []byte(salt)
	if atIndex != -1 {
		mailbox = s[:atIndex]
	}
	domain := string(s[atIndex+1:])
	if domain == excludeDomain {
		return s
	} else {
		scrambleLength := len(mailbox) + len(domain)
		s = make([]byte, scrambleLength+len(replacementDomain))
		copy(s, mailbox)
		copy(s[len(mailbox):], domain)
		// scrambleBytes is in-place, but may return string shorter than input.
		scrambleBytes(s[:scrambleLength], salt)
		copy(s[scrambleLength:], replacementDomain)
		// So final len(mailbox) may be shorter than whole allocated string.
		return s[:scrambleLength+len(replacementDomain)]
	}
}

// Supports array of emails in format {email1,email2}
func scrambleEmail(s []byte, excludeDomain string, replacementDomain string, salt string) []byte {
	if len(s) < 2 {
		// panic("scrambleEmail: input is too small: '" + string(s) + "'")
		return s
	}
	if s[0] != '{' && s[len(s)-1] != '}' {
		return scrambleOneEmail(s, excludeDomain, replacementDomain, salt)
	}
	parts := bytes.Split(s[1:len(s)-1], []byte{','})
	partsNew := make([][]byte, len(parts))
	outLength := 2 + len(parts) - 1
	for i, bs := range parts {
		partsNew[i] = scrambleOneEmail(bs, excludeDomain, replacementDomain, salt)
		outLength += len(partsNew[i])
	}
	s = make([]byte, outLength)
	s[0] = '{'
	s[len(s)-1] = '}'
	copy(s[1:len(s)-1], bytes.Join(partsNew, []byte{','}))
	return s
}

type AtEmailTransformer struct {
	replacementDomain string
	excludeDomain     string
	salt              string
}

var (
	atEmailParams = []Parameter{
		{
			Name:          "replacement_domain",
			SupportedType: "string",
			Default:       "@crypt.com",
			Dynamic:       false,
			Required:      false,
		},
		{
			Name:          "exclude_domain",
			SupportedType: "string",
			Default:       "",
			Dynamic:       false,
			Required:      false,
		},
		{
			Name:          "salt",
			SupportedType: "string",
			Default:       "defaultsalt",
			Dynamic:       false,
			Required:      false,
		},
	}
	atEmailCompatibleTypes = []SupportedDataType{
		StringDataType,
		CitextDataType,
	}
)

func NewAtEmailTransformer(params ParameterValues) (*AtEmailTransformer, error) {
	replacementDomain, err := FindParameterWithDefault(params, "replacement_domain", "@crypt.com")
	if err != nil {
		return nil, fmt.Errorf("replacement_domain: replacement_domain must be a string: %w", err)
	}
	excludeDomain, err := FindParameterWithDefault(params, "exclude_domain", "")
	if err != nil {
		return nil, fmt.Errorf("exclude_domain: exclude_domain must be a string: %w", err)
	}
	salt, err := FindParameterWithDefault(params, "salt", "defaultsalt")
	if err != nil {
		return nil, fmt.Errorf("salt: salt must be a string: %w", err)
	}

	return &AtEmailTransformer{
		replacementDomain: replacementDomain,
		excludeDomain: excludeDomain,
		salt:              salt,
	}, nil
}

func (st *AtEmailTransformer) Transform(_ context.Context, v Value) (any, error) {
	switch str := v.TransformValue.(type) {
	case string:
		return st.transform(str), nil
	case []byte:
		return st.transform(string(str)), nil
	default:
		return v, fmt.Errorf("expected string, got %T: %w", v, ErrUnsupportedValueType)
	}
}

func (st *AtEmailTransformer) transform(str string) string {
	b := scrambleEmail([]byte(str), st.excludeDomain, st.replacementDomain, st.salt)
	return string(b)
}

func (st *AtEmailTransformer) CompatibleTypes() []SupportedDataType {
	return atEmailCompatibleTypes
}

func (st *AtEmailTransformer) Type() TransformerType {
	return AtEmail
}

func AtEmailTransformerDefinition() *Definition {
	return &Definition{
		SupportedTypes: atEmailCompatibleTypes,
		Parameters:     atEmailParams,
	}
}

func FuncMap() template.FuncMap {
	functions := template.FuncMap{
		"atEmailScramble": func(excludeDomain string, replacementDomain string, salt string, value string) string {
			return string(scrambleEmail([]byte(value), excludeDomain, replacementDomain, salt))
		},
	}
	tm := make(template.FuncMap)
	maps.Copy(tm, functions)
	return tm
}
