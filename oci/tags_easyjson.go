// Code generated by easyjson for marshaling/unmarshaling. DO NOT EDIT.

package oci

import (
	json "encoding/json"
	easyjson "github.com/mailru/easyjson"
	jlexer "github.com/mailru/easyjson/jlexer"
	jwriter "github.com/mailru/easyjson/jwriter"
)

// suppress unused package warning
var (
	_ *json.RawMessage
	_ *jlexer.Lexer
	_ *jwriter.Writer
	_ easyjson.Marshaler
)

func easyjson2aad9953DecodeGithubComRicardobranco777RegviewOci(in *jlexer.Lexer, out *TagList) {
	isTopLevel := in.IsStart()
	if in.IsNull() {
		if isTopLevel {
			in.Consumed()
		}
		in.Skip()
		return
	}
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(true)
		in.WantColon()
		if in.IsNull() {
			in.Skip()
			in.WantComma()
			continue
		}
		switch key {
		case "tags":
			if in.IsNull() {
				in.Skip()
				out.Tags = nil
			} else {
				in.Delim('[')
				if out.Tags == nil {
					if !in.IsDelim(']') {
						out.Tags = make([]string, 0, 4)
					} else {
						out.Tags = []string{}
					}
				} else {
					out.Tags = (out.Tags)[:0]
				}
				for !in.IsDelim(']') {
					var v1 string
					v1 = string(in.String())
					out.Tags = append(out.Tags, v1)
					in.WantComma()
				}
				in.Delim(']')
			}
		default:
			in.SkipRecursive()
		}
		in.WantComma()
	}
	in.Delim('}')
	if isTopLevel {
		in.Consumed()
	}
}
func easyjson2aad9953EncodeGithubComRicardobranco777RegviewOci(out *jwriter.Writer, in TagList) {
	out.RawByte('{')
	first := true
	_ = first
	{
		const prefix string = ",\"tags\":"
		out.RawString(prefix[1:])
		if in.Tags == nil && (out.Flags&jwriter.NilSliceAsEmpty) == 0 {
			out.RawString("null")
		} else {
			out.RawByte('[')
			for v2, v3 := range in.Tags {
				if v2 > 0 {
					out.RawByte(',')
				}
				out.String(string(v3))
			}
			out.RawByte(']')
		}
	}
	out.RawByte('}')
}

// MarshalJSON supports json.Marshaler interface
func (v TagList) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	easyjson2aad9953EncodeGithubComRicardobranco777RegviewOci(&w, v)
	return w.Buffer.BuildBytes(), w.Error
}

// MarshalEasyJSON supports easyjson.Marshaler interface
func (v TagList) MarshalEasyJSON(w *jwriter.Writer) {
	easyjson2aad9953EncodeGithubComRicardobranco777RegviewOci(w, v)
}

// UnmarshalJSON supports json.Unmarshaler interface
func (v *TagList) UnmarshalJSON(data []byte) error {
	r := jlexer.Lexer{Data: data}
	easyjson2aad9953DecodeGithubComRicardobranco777RegviewOci(&r, v)
	return r.Error()
}

// UnmarshalEasyJSON supports easyjson.Unmarshaler interface
func (v *TagList) UnmarshalEasyJSON(l *jlexer.Lexer) {
	easyjson2aad9953DecodeGithubComRicardobranco777RegviewOci(l, v)
}