//go:build darwin || windows

package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// 趣味反馈只由操作触发，结束后不再请求动画帧。
var playfulSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 360 * time.Millisecond, Bounce: .22}}
var navigationSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond, Bounce: .12}}

type navigationSelectionID string

// 同一块选中底色在条目之间移动，文字和点击区域始终留在原位。
func navigationSelection(c *ui.Context, group string, selected bool, p palette) {
	if selected {
		ui.Box(c.Key("selection")).Absolute().Fill().Radius(8).Background(p.selected).
			Transition(ui.ElementTransition{Position: true, Size: true, Animation: &navigationSpring, Match: navigationSelectionID(group)})
	}
}

func playfulIcon(c *ui.Context, button ui.Element, icon *ui.SVG, ink ui.Color) {
	if icon == iconHeart || icon == iconHeartFilled {
		favoriteArtwork(c, button, icon == iconHeartFilled, ink)
		return
	}
	pose := ui.Pose{}
	if !c.Preferences().ReduceMotion {
		if button.Hovered() || button.FocusVisible() {
			pose.Scale = 1.10
		}
		if button.Pressed() {
			pose.Scale = .84
		}
	}
	ui.Icon(c, icon).Size(17, 17).TextColor(ink).AnimatePose(pose, playfulSpring)
}

type favoriteReaction struct {
	selected bool
	run      uint64
}

func favoriteArtwork(c *ui.Context, button ui.Element, selected bool, ink ui.Color) {
	favoriteTicket(c, button, selected, ink)
}

func coverPlayCue(c *ui.Context, button ui.Element) {
	shown := button.Hovered() || button.FocusVisible()
	opacity := float32(0)
	pose := ui.Pose{Y: 10, Scale: .70, Rotate: -12}
	if shown {
		opacity = 1
		pose = ui.Pose{}
	}
	if button.Pressed() {
		pose = ui.Pose{Y: 1, Scale: .88, Rotate: -6}
	}
	if c.Preferences().ReduceMotion {
		pose = ui.Pose{}
	}
	ui.Column(c.Key("play-cue")).Absolute().Fill().Center().Radius(9).
		Background(ui.RGBA(0, 0, 0, .10)).Opacity(button.Animate("play", opacity, 130*time.Millisecond)).Children(func() {
		ui.Box(c).Size(36, 36).Radius(18).Background(ui.RGBA(0, 0, 0, .48)).Center().
			AnimatePose(pose, playfulSpring).Children(func() {
			ui.Icon(c, iconPlay).Size(17, 17).TextColor(ui.Hex("#ffffff"))
		})
	})
}

// SVG 在打包时栅格化一次，动效帧只合成纹理，不反复细分角色路径。
var mascotBody = bitmapAsset("mascot/body.png")

type mascotReaction struct {
	hovered   bool
	hop, wave uint64
}

var mascotHop = []ui.Keyframe{
	{Value: 0},
	{At: 90 * time.Millisecond, Value: 2, Ease: ui.EaseOut},
	{At: 230 * time.Millisecond, Value: -9, Ease: ui.EaseOut},
	{At: 390 * time.Millisecond, Value: 1.5, Ease: ui.EaseIn},
	{At: 490 * time.Millisecond, Value: -2, Ease: ui.EaseOut},
	{At: 650 * time.Millisecond, Value: 0, Ease: ui.EaseOut},
}
var mascotSquash = []ui.Keyframe{
	{Value: 0},
	{At: 90 * time.Millisecond, Value: .10, Ease: ui.EaseOut},
	{At: 230 * time.Millisecond, Value: -.06, Ease: ui.EaseOut},
	{At: 390 * time.Millisecond, Value: .07, Ease: ui.EaseIn},
	{At: 650 * time.Millisecond, Value: 0, Ease: ui.EaseOut},
}
var mascotWave = []ui.Keyframe{
	{Value: 0},
	{At: 130 * time.Millisecond, Value: -18, Ease: ui.EaseOut},
	{At: 260 * time.Millisecond, Value: 12, Ease: ui.EaseInOut},
	{At: 390 * time.Millisecond, Value: -14, Ease: ui.EaseInOut},
	{At: 560 * time.Millisecond, Value: 0, Ease: ui.EaseOut},
}

// 底板固定，角色短暂蹦跳和左右应援；动作完成后恢复安静。
func playfulMascot(c *ui.Context) {
	p := colors(c.Theme())
	b := ui.ButtonBase(c.Key("mascot")).Size(82, 82).Radius(20).Background(p.soft).Border(.75, p.line).Center().Label("和高松灯打个招呼").Tooltip("一起打 call").FocusRing(true)
	state := ui.Local(b, "reaction", func() mascotReaction { return mascotReaction{} })
	if b.Clicked() {
		state.hop++
		state.wave++
	}
	hovered := b.Hovered() || b.FocusVisible()
	if hovered && !state.hovered {
		state.wave++
	}
	state.hovered = hovered
	hop := b.Keyframes("hop", mascotHop, ui.KeyframeOptions{Run: state.hop})
	squash := b.Keyframes("squash", mascotSquash, ui.KeyframeOptions{Run: state.hop})
	wave := b.Keyframes("wave", mascotWave, ui.KeyframeOptions{Run: state.wave})
	lean := ui.Pose{}
	if hovered && !c.Preferences().ReduceMotion {
		lean.Rotate = -4
	}
	b.Children(func() {
		ui.Box(c).Size(82, 82).Shrink(0).TransformOrigin(.5, .80).Scale(1+squash, 1-squash).Rotate(wave*.22).Translate(0, hop).
			AnimatePose(lean, playfulSpring).Children(func() {
			ui.Image(c, mascotBody).Size(82, 82)
		})
	})
}
