package main

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// A shortcut of the user's own for the 想法一瞬 panel, set in the settings
// by its modifiers and its key, rather than by pressing it: a combination
// with ⌥ types a character instead, which an input would not report.

// shortcutDraft is a shortcut being set.
type shortcutDraft struct {
	open                  bool
	ctrl, alt, shift, cmd bool
	key                   string
	err                   string
}

// shortcutKeys are the keys a shortcut of the user's can end in.
var shortcutKeys = func() []string {
	var keys []string
	for r := 'A'; r <= 'Z'; r++ {
		keys = append(keys, string(r))
	}
	for r := '0'; r <= '9'; r++ {
		keys = append(keys, string(r))
	}
	keys = append(keys, "Space")
	for n := 1; n <= 12; n++ {
		keys = append(keys, fmt.Sprintf("F%d", n))
	}
	return keys
}()

// draftOf reads an accelerator into its modifiers and key.
func draftOf(acc string) shortcutDraft {
	var d shortcutDraft
	parts := strings.Split(acc, "+")
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "Ctrl":
			d.ctrl = true
		case "Alt":
			d.alt = true
		case "Shift":
			d.shift = true
		case "Cmd":
			d.cmd = true
		case "CmdOrCtrl":
			if runtime.GOOS == "darwin" {
				d.cmd = true
			} else {
				d.ctrl = true
			}
		}
	}
	if k := parts[len(parts)-1]; slices.Contains(shortcutKeys, k) {
		d.key = k
	}
	return d
}

// accelerator is the shortcut, "" until it has a key.
func (d shortcutDraft) accelerator() string {
	if d.key == "" {
		return ""
	}
	var parts []string
	for _, m := range []struct {
		on   bool
		name string
	}{{d.ctrl, "Ctrl"}, {d.alt, "Alt"}, {d.shift, "Shift"}, {d.cmd, "Cmd"}} {
		if m.on {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, d.key), "+")
}

// shortcutProblem says why a shortcut would be a bad one, "" for none: one
// without ⌃, ⌥ or ⌘ would take a key, or ⇧ and a key, from typing, but
// for the function keys.
func (a *app) shortcutProblem(d shortcutDraft) string {
	switch {
	case d.key == "":
		return a.tr("选一个按键", "Choose a key")
	case !d.ctrl && !d.alt && !d.cmd && !strings.HasPrefix(d.key, "F"):
		return a.tr("至少要有 ⌃、⌥ 或 ⌘ 其中一个，否则会影响打字", "Use ⌃, ⌥ or ⌘ too, or it would get in the way of typing")
	case d.cmd && !d.ctrl && !d.alt && !d.shift && d.key != "Space":
		return a.tr("单独的 ⌘ 加字母会和应用里的快捷键冲突，再加一个修饰键", "⌘ and a letter alone is an app's shortcut: add another modifier")
	}
	return ""
}

// ideaShortcutChoice is what the settings' menu shows of the shortcut.
func (a *app) ideaShortcutChoice() string {
	if a.set.IdeaShortcut == "" {
		return defaultIdeaShortcut
	}
	return a.set.IdeaShortcut
}

// ideaShortcutSetting is the settings' item of the shortcut: a menu of
// shortcuts, then the fields of the user's own.
func (a *app) ideaShortcutSetting(c *ui.Context) {
	t := c.Theme()
	d := &a.shortcutDraft
	ui.Column(c).Gap(8).AlignItems(ui.End).Children(func() {
		v := a.ideaShortcutChoice()
		if d.open {
			v = ideaShortcutCustom
		}
		opts := make([]components.NativeSelectOption[string], 0, len(ideaShortcuts)+3)
		for k, acc := range ideaShortcuts {
			label := acceleratorText(acc)
			if k == 0 {
				label += a.tr("（推荐）", " (recommended)")
			}
			opts = append(opts, components.NativeSelectOption[string]{Value: acc, Label: label})
		}
		if cur := a.ideaShortcutChoice(); cur != ideaShortcutOff && !slices.Contains(ideaShortcuts, cur) {
			opts = append(opts, components.NativeSelectOption[string]{Value: cur, Label: acceleratorText(cur)})
		}
		opts = append(opts,
			components.NativeSelectOption[string]{Value: ideaShortcutCustom, Label: a.tr("自定义…", "Custom…")},
			components.NativeSelectOption[string]{Value: ideaShortcutOff, Label: a.tr("关闭", "Off")})
		sel := components.NativeSelect(c, &v, opts)
		sel.Trigger.Label(a.tr("想法一瞬快捷键", "Idea Flash shortcut")).Width(200)
		if sel.Changed() {
			if v == ideaShortcutCustom {
				*d = draftOf(a.ideaShortcut())
				d.open = true
			} else {
				d.open = false
				a.setIdeaShortcut(v)
			}
		}
		if !d.open {
			return
		}
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			mods := []struct {
				on    *bool
				sym   string
				label string
			}{{&d.ctrl, "⌃", "Control"}, {&d.alt, "⌥", "Option"}, {&d.shift, "⇧", "Shift"}}
			if runtime.GOOS == "darwin" {
				mods = append(mods, struct {
					on    *bool
					sym   string
					label string
				}{&d.cmd, "⌘", "Command"})
			} else {
				mods[0].sym, mods[1].sym, mods[1].label = "Ctrl", "Alt", "Alt"
			}
			for _, m := range mods {
				variant := components.ButtonOutline
				if *m.on {
					variant = components.ButtonPrimary
				}
				if components.Button(c, m.sym, variant).Key("mod:" + m.label).Label(m.label).Clicked() {
					*m.on = !*m.on
					d.err = ""
				}
			}
			ui.Text(c, "+").TextColor(t.TextMuted)
			keys := make([]components.NativeSelectOption[string], 0, len(shortcutKeys)+1)
			keys = append(keys, components.NativeSelectOption[string]{Value: "", Label: a.tr("按键", "Key")})
			for _, k := range shortcutKeys {
				keys = append(keys, components.NativeSelectOption[string]{Value: k, Label: k})
			}
			ks := components.NativeSelect(c, &d.key, keys)
			ks.Trigger.Label(a.tr("按键", "Key")).Width(90)
			if ks.Changed() {
				d.err = ""
			}
		})
		problem := a.shortcutProblem(*d)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			msg, color := problem, t.TextMuted
			if d.err != "" {
				msg, color = d.err, t.Danger
			} else if problem == "" {
				msg = a.tr("将使用 ", "Will use ") + acceleratorText(d.accelerator())
			}
			ui.Text(c, msg).FontSize(12).TextColor(color).Wrap().MaxWidth(260)
			if components.Button(c, a.tr("取消", "Cancel"), components.ButtonGhost).Clicked() {
				d.open = false
			}
			if components.Button(c, a.tr("使用", "Use"), components.ButtonPrimary).Disabled(problem != "").Clicked() {
				if err := a.setIdeaShortcut(d.accelerator()); err != nil {
					d.err = acceleratorText(d.accelerator()) + a.tr(" 已被占用，请换一个", " is taken; choose another")
				} else {
					d.open = false
				}
			}
		})
	})
}
