//go:build windows && (amd64 || arm64)

package player

type renderer struct{ surface *Surface }

func (p *Player) initRenderer(_ uintptr, s *Surface) error {
	p.renderer = &renderer{surface: s}
	return nil
}

func (*renderer) close() {}
