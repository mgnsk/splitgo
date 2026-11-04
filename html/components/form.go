// Package components contains reusable view components.
package components

import (
	"net/url"

	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
)

// FieldProps is props for a form field.
type FieldProps struct {
	Label       string
	Name        string
	Type        string
	Value       string
	Placeholder string
	Hint        string
	Errors      url.Values
	Required    bool
	Attrs       []Node
	// AfterInput is rendered right after the input, positioned against it,
	// e.g. a suggestion list.
	AfterInput Node
}

// Field renders a labeled input with an optional error message.
func Field(p FieldProps) Node {
	errMsg := p.Errors.Get(p.Name)
	id := "f-" + p.Name

	typ := p.Type
	if typ == "" {
		typ = "text"
	}

	var input Node = Input(
		ID(id),
		Name(p.Name),
		Type(typ),
		Value(p.Value),
		If(p.Placeholder != "", Placeholder(p.Placeholder)),
		If(p.Required, Required()),
		If(errMsg != "", Group{Class("invalid"), Aria("invalid", "true")}),
		Group(p.Attrs),
	)
	if p.AfterInput != nil {
		input = Div(Class("combo"), input, p.AfterInput)
	}

	return Div(Class("field"),
		If(p.Label != "", Label(For(id), Text(p.Label))),
		input,
		If(p.Hint != "" && errMsg == "", P(Class("hint"), Text(p.Hint))),
		ErrorText(errMsg),
	)
}

// SelectOption is a select option.
type SelectOption struct {
	Value string
	Label string
}

// SelectProps is props for a select field.
type SelectProps struct {
	Label    string
	Name     string
	Value    string
	Options  []SelectOption
	Errors   url.Values
	Attrs    []Node
	ExtraCSS string
}

// SelectField renders a labeled select.
func SelectField(p SelectProps) Node {
	errMsg := p.Errors.Get(p.Name)
	id := "f-" + p.Name

	return Div(Classes{"field": true, p.ExtraCSS: p.ExtraCSS != ""},
		If(p.Label != "", Label(For(id), Text(p.Label))),
		El("select",
			ID(id),
			Name(p.Name),
			If(errMsg != "", Class("invalid")),
			Group(p.Attrs),
			Map(p.Options, func(o SelectOption) Node {
				return El("option", Value(o.Value), If(o.Value == p.Value, Selected()), Text(o.Label))
			}),
		),
		ErrorText(errMsg),
	)
}

// ErrorText renders an error message if not empty.
func ErrorText(msg string) Node {
	return If(msg != "", P(Class("error-text"), Role("alert"), Text(msg)))
}

// SubmitButton renders a primary submit button.
func SubmitButton(text string, block bool) Node {
	return Button(Type("submit"), Classes{"btn": true, "btn-primary": true, "btn-block": block}, Text(text))
}
