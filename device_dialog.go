package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/golangsnmp/mibsh/internal/profile"
	"github.com/golangsnmp/mibsh/internal/snmp"
)

type dialogField int

const (
	fieldTarget dialogField = iota
	fieldCommunity
	fieldVersion
	fieldSecLevel
	fieldUsername
	fieldAuthProto
	fieldAuthPass
	fieldPrivProto
	fieldPrivPass
)

type dialogSection int

const (
	sectionProfiles dialogSection = iota
	sectionFields
)

// deviceDialogSubmitMsg carries a device from the dialog to initiate connection.
type deviceDialogSubmitMsg struct {
	device profile.Device
}

// deviceDialogDeleteMsg requests removal of a saved profile.
type deviceDialogDeleteMsg struct {
	name string
}

// dialogInput wraps either a textinput or a select behind uniform accessors.
type dialogInput struct {
	text *textinput.Model
	sel  *selectModel
}

func (di dialogInput) Focus() tea.Cmd {
	if di.sel != nil {
		return di.sel.Focus()
	}
	return di.text.Focus()
}

func (di dialogInput) Blur() {
	if di.sel != nil {
		di.sel.Blur()
	} else {
		di.text.Blur()
	}
}

func (di dialogInput) Value() string {
	if di.sel != nil {
		return di.sel.Value()
	}
	return di.text.Value()
}

func (di dialogInput) SetValue(v string) {
	if di.sel != nil {
		di.sel.SetValue(v)
	} else {
		di.text.SetValue(v)
	}
}

func (di dialogInput) Update(msg tea.Msg) tea.Cmd {
	if di.sel != nil {
		*di.sel, _ = di.sel.Update(msg)
		return nil
	}
	var cmd tea.Cmd
	*di.text, cmd = di.text.Update(msg)
	return cmd
}

func (di dialogInput) activeView() string {
	if di.sel != nil {
		return di.sel.View()
	}
	return di.text.View()
}

func (di dialogInput) isPassword() bool {
	if di.text != nil {
		return di.text.EchoMode == textinput.EchoPassword
	}
	return false
}

// deviceDialogModel is a modal dialog for SNMP connection parameters,
// with optional saved profile selection.
type deviceDialogModel struct {
	// Saved profiles
	profiles   []profile.Device
	profileIdx int // cursor position in profile list

	// Manual fields
	target    textinput.Model
	community textinput.Model
	version   selectModel
	secLevel  selectModel
	username  textinput.Model
	authProto selectModel
	authPass  textinput.Model
	privProto selectModel
	privPass  textinput.Model

	section dialogSection // which section has focus
	focused dialogField   // which field is focused (when section==sectionFields)
	err     string
}

func newDeviceDialog(cfg appConfig, profiles []profile.Device) deviceDialogModel {
	cs := textinput.CursorStyle{
		Color: palette.Primary,
		Shape: tea.CursorBar,
		Blink: true,
	}

	mkInput := func(placeholder string, charLimit int, value string) textinput.Model {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = placeholder
		ti.CharLimit = charLimit
		s := ti.Styles()
		s.Cursor = cs
		ti.SetStyles(s)
		if value != "" {
			ti.SetValue(value)
		}
		return ti
	}

	target := mkInput("host or host:port", 256, cfg.target)
	community := mkInput("public", 128, cfg.community)
	if community.Value() == "" {
		community.SetValue("public")
	}

	version := newSelect([]string{"1", "2c", "3"})
	if cfg.version != "" {
		version.SetValue(cfg.version)
	} else {
		version.SetValue("2c")
	}

	secLevel := newSelect([]string{"noAuthNoPriv", "authNoPriv", "authPriv"})
	username := mkInput("username", 128, "")
	authProto := newSelect([]string{"MD5", "SHA", "SHA224", "SHA256", "SHA384", "SHA512"})
	authProto.SetValue("SHA")
	authPass := mkInput("passphrase", 128, "")
	authPass.EchoMode = textinput.EchoPassword
	authPass.EchoCharacter = '*'
	privProto := newSelect([]string{"DES", "AES", "AES192", "AES256"})
	privProto.SetValue("AES")
	privPass := mkInput("passphrase", 128, "")
	privPass.EchoMode = textinput.EchoPassword
	privPass.EchoCharacter = '*'

	d := deviceDialogModel{
		profiles:  profiles,
		target:    target,
		community: community,
		version:   version,
		secLevel:  secLevel,
		username:  username,
		authProto: authProto,
		authPass:  authPass,
		privProto: privProto,
		privPass:  privPass,
		focused:   fieldTarget,
	}

	if len(profiles) > 0 {
		d.section = sectionProfiles
	} else {
		d.section = sectionFields
	}

	return d
}

// isV3 returns true if the current version field value indicates SNMPv3.
func (d *deviceDialogModel) isV3() bool {
	return d.version.Value() == "3"
}

// visibleFields returns the list of active fields based on version and security level.
func (d *deviceDialogModel) visibleFields() []dialogField {
	if !d.isV3() {
		return []dialogField{fieldTarget, fieldCommunity, fieldVersion}
	}
	switch d.secLevel.Value() {
	case "noAuthNoPriv":
		return []dialogField{
			fieldTarget, fieldVersion,
			fieldSecLevel, fieldUsername,
		}
	case "authNoPriv":
		return []dialogField{
			fieldTarget, fieldVersion,
			fieldSecLevel, fieldUsername,
			fieldAuthProto, fieldAuthPass,
		}
	default: // authPriv
		return []dialogField{
			fieldTarget, fieldVersion,
			fieldSecLevel, fieldUsername,
			fieldAuthProto, fieldAuthPass,
			fieldPrivProto, fieldPrivPass,
		}
	}
}

func (d *deviceDialogModel) fieldInput(f dialogField) dialogInput {
	switch f {
	case fieldTarget:
		return dialogInput{text: &d.target}
	case fieldCommunity:
		return dialogInput{text: &d.community}
	case fieldVersion:
		return dialogInput{sel: &d.version}
	case fieldSecLevel:
		return dialogInput{sel: &d.secLevel}
	case fieldUsername:
		return dialogInput{text: &d.username}
	case fieldAuthProto:
		return dialogInput{sel: &d.authProto}
	case fieldAuthPass:
		return dialogInput{text: &d.authPass}
	case fieldPrivProto:
		return dialogInput{sel: &d.privProto}
	case fieldPrivPass:
		return dialogInput{text: &d.privPass}
	}
	return dialogInput{}
}

func fieldLabel(f dialogField) string {
	switch f {
	case fieldTarget:
		return "Target:"
	case fieldCommunity:
		return "Community:"
	case fieldVersion:
		return "Version:"
	case fieldSecLevel:
		return "Security:"
	case fieldUsername:
		return "Username:"
	case fieldAuthProto:
		return "Auth Proto:"
	case fieldAuthPass:
		return "Auth Pass:"
	case fieldPrivProto:
		return "Priv Proto:"
	case fieldPrivPass:
		return "Priv Pass:"
	}
	return ""
}

func (d *deviceDialogModel) focusCmd() tea.Cmd {
	if d.section == sectionProfiles {
		return nil
	}
	return d.fieldInput(d.focused).Focus()
}

func (d *deviceDialogModel) blurAll() {
	d.target.Blur()
	d.community.Blur()
	d.version.Blur()
	d.secLevel.Blur()
	d.username.Blur()
	d.authProto.Blur()
	d.authPass.Blur()
	d.privProto.Blur()
	d.privPass.Blur()
}

func (d *deviceDialogModel) cycleForward() tea.Cmd {
	d.blurAll()
	fields := d.visibleFields()
	cur := slices.Index(fields, d.focused)
	next := (cur + 1) % len(fields)
	d.focused = fields[next]
	return d.focusCmd()
}

func (d *deviceDialogModel) cycleBackward() tea.Cmd {
	d.blurAll()
	fields := d.visibleFields()
	cur := slices.Index(fields, d.focused)
	if cur <= 0 {
		// At first field, go to profile list if available
		if len(d.profiles) > 0 {
			d.section = sectionProfiles
			return nil
		}
		// Wrap to last field
		d.focused = fields[len(fields)-1]
	} else {
		d.focused = fields[cur-1]
	}
	return d.focusCmd()
}

// focusFieldAt focuses a field by its dialogField value.
func (d *deviceDialogModel) focusFieldAt(f dialogField) tea.Cmd {
	fields := d.visibleFields()
	for _, vf := range fields {
		if vf == f {
			d.blurAll()
			d.section = sectionFields
			d.focused = f
			return d.focusCmd()
		}
	}
	return nil
}

// fillFromProfile populates the manual fields from a saved profile.
func (d *deviceDialogModel) fillFromProfile(p profile.Device) {
	d.target.SetValue(p.Target)
	d.version.SetValue(p.Version)
	d.community.SetValue(p.Community)
	if d.community.Value() == "" {
		d.community.SetValue("public")
	}
	d.secLevel.SetValue(p.SecurityLevel)
	d.username.SetValue(p.Username)
	d.authProto.SetValue(p.AuthProto)
	d.authPass.SetValue(p.AuthPass)
	d.privProto.SetValue(p.PrivProto)
	d.privPass.SetValue(p.PrivPass)
}

func (d *deviceDialogModel) validate() error {
	if strings.TrimSpace(d.target.Value()) == "" {
		return errors.New("target is required")
	}
	if _, err := snmp.ParseVersion(d.version.Value()); err != nil {
		return err
	}
	if d.isV3() {
		if strings.TrimSpace(d.username.Value()) == "" {
			return errors.New("username is required for v3")
		}
	}
	return nil
}

func (d *deviceDialogModel) device() profile.Device {
	target := strings.TrimSpace(d.target.Value())
	p := profile.Device{
		Name: target,
		Profile: snmp.Profile{
			Target:  target,
			Version: d.version.Value(),
		},
	}
	if d.isV3() {
		p.SecurityLevel = d.secLevel.Value()
		p.Username = strings.TrimSpace(d.username.Value())
		p.AuthProto = d.authProto.Value()
		p.AuthPass = d.authPass.Value()
		p.PrivProto = d.privProto.Value()
		p.PrivPass = d.privPass.Value()
	} else {
		p.Community = strings.TrimSpace(d.community.Value())
	}
	return p
}

func (d *deviceDialogModel) submitCmd() tea.Cmd {
	dev := d.device()
	return func() tea.Msg {
		return deviceDialogSubmitMsg{device: dev}
	}
}

func (d *deviceDialogModel) update(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		return nil, true
	}

	if d.section == sectionProfiles {
		return d.updateProfiles(msg)
	}
	return d.updateFields(msg)
}

func (d *deviceDialogModel) updateProfiles(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "j", "down":
		if d.profileIdx < len(d.profiles)-1 {
			d.profileIdx++
		}
		return nil, false
	case "k", "up":
		if d.profileIdx > 0 {
			d.profileIdx--
		}
		return nil, false
	case "enter":
		if d.profileIdx >= 0 && d.profileIdx < len(d.profiles) {
			dev := d.profiles[d.profileIdx]
			return func() tea.Msg {
				return deviceDialogSubmitMsg{device: dev}
			}, true
		}
		return nil, false
	case "tab":
		// Switch to fields, pre-fill from selected profile
		if d.profileIdx >= 0 && d.profileIdx < len(d.profiles) {
			d.fillFromProfile(d.profiles[d.profileIdx])
		}
		d.section = sectionFields
		d.focused = d.visibleFields()[0]
		return d.focusCmd(), false
	case "delete", "backspace":
		if d.profileIdx >= 0 && d.profileIdx < len(d.profiles) {
			name := d.profiles[d.profileIdx].Name
			d.profiles = append(d.profiles[:d.profileIdx], d.profiles[d.profileIdx+1:]...)
			if d.profileIdx >= len(d.profiles) && d.profileIdx > 0 {
				d.profileIdx--
			}
			if len(d.profiles) == 0 {
				d.section = sectionFields
				d.focused = d.visibleFields()[0]
				return tea.Batch(func() tea.Msg {
					return deviceDialogDeleteMsg{name: name}
				}, d.focusCmd()), false
			}
			return func() tea.Msg {
				return deviceDialogDeleteMsg{name: name}
			}, false
		}
		return nil, false
	}
	return nil, false
}

func (d *deviceDialogModel) updateFields(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "tab":
		return d.cycleForward(), false
	case "shift+tab":
		return d.cycleBackward(), false
	case "enter":
		if err := d.validate(); err != nil {
			d.err = err.Error()
			return nil, false
		}
		return d.submitCmd(), true
	}

	// Forward to focused input
	di := d.fieldInput(d.focused)
	cmd := di.Update(msg)
	d.err = ""
	return cmd, false
}

// fieldStartLine returns the content line number where manual fields begin.
func (d *deviceDialogModel) fieldStartLine() int {
	if len(d.profiles) > 0 {
		// title + blank + header + profiles + blank
		return 3 + len(d.profiles)
	}
	return 2 // title + blank
}

// lineToField maps a content Y offset to a dialogField.
// Returns false if the line does not correspond to a field.
func (d *deviceDialogModel) lineToField(line int) (dialogField, bool) {
	start := d.fieldStartLine()
	fields := d.visibleFields()
	idx := line - start
	if idx >= 0 && idx < len(fields) {
		return fields[idx], true
	}
	return 0, false
}

// lineToProfile maps a content Y offset to a profile index, or -1.
func (d *deviceDialogModel) lineToProfile(line int) int {
	if len(d.profiles) == 0 {
		return -1
	}
	// Profiles start at line 3 (title + blank + header)
	idx := line - 3
	if idx >= 0 && idx < len(d.profiles) {
		return idx
	}
	return -1
}

func (d *deviceDialogModel) view() string {
	var b strings.Builder
	bg := palette.BgLighter

	title := styles.Dialog.Title.Background(bg).Render("Connect to Device")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Profile list
	if len(d.profiles) > 0 {
		b.WriteString(styles.Header.Info.Background(bg).Render("Saved Profiles"))
		b.WriteByte('\n')
		for i, p := range d.profiles {
			var indicator string
			if d.section == sectionProfiles && i == d.profileIdx {
				indicator = styles.Tree.FocusBorder.Background(bg).Render(BorderThick) + " "
			} else {
				indicator = "  "
			}
			name := styles.Value.Background(bg).Render(p.Name)
			summary := styles.Label.Background(bg).Render("  (" + p.Summary() + ")")
			b.WriteString(indicator + name + summary)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	// Manual fields
	labelW := 12
	if d.isV3() {
		labelW = 14
	}

	fields := d.visibleFields()
	for _, f := range fields {
		di := d.fieldInput(f)
		active := d.section == sectionFields && d.focused == f
		lbl := styles.Label.Background(bg).Render(fmt.Sprintf("%-*s", labelW, fieldLabel(f)))
		b.WriteString(lbl)
		if active {
			b.WriteString(di.activeView())
		} else {
			val := di.Value()
			if di.isPassword() && val != "" {
				val = strings.Repeat("*", len(val))
			}
			b.WriteString(styles.Value.Background(bg).Render(val))
		}
		b.WriteByte('\n')
	}

	if d.err != "" {
		b.WriteByte('\n')
		b.WriteString(styles.Status.ErrorMsg.Background(bg).Render(d.err))
	}

	b.WriteByte('\n')
	const keyW = 7
	keyStyle := styles.Label.Background(bg).Width(keyW)
	valStyle := styles.Value.Background(bg)
	if len(d.profiles) > 0 && d.section == sectionProfiles {
		b.WriteString(keyStyle.Render("j/k") + valStyle.Render("select profile") + "\n")
		b.WriteString(keyStyle.Render("enter") + valStyle.Render("connect") + "\n")
		b.WriteString(keyStyle.Render("tab") + valStyle.Render("edit fields") + "\n")
		b.WriteString(keyStyle.Render("del") + valStyle.Render("remove profile") + "\n")
		b.WriteString(keyStyle.Render("esc") + valStyle.Render("cancel"))
	} else {
		b.WriteString(keyStyle.Render("tab") + valStyle.Render("next field") + "\n")
		b.WriteString(keyStyle.Render("enter") + valStyle.Render("connect") + "\n")
		b.WriteString(keyStyle.Render("esc") + valStyle.Render("cancel"))
	}

	dialogW := 46
	if d.isV3() {
		dialogW = 52
	}
	content := padContentBg(b.String(), bg)
	return lipgloss.NewStyle().Width(dialogW).Background(bg).Render(content)
}
