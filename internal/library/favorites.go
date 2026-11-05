package library

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"live-mygo/internal/platforms"
)

func FavoriteKey(room platforms.Room) string { return string(room.Platform) + ":" + room.ID }

func (s *State) Details(room platforms.Room) FavoriteDetails {
	return s.FavoriteDetails[FavoriteKey(room)]
}

func (s *State) SetFavoriteDetails(room platforms.Room, details FavoriteDetails) error {
	if !s.Has(room) {
		return errors.New("请先关注这个直播间")
	}
	var err error
	details, err = normalizeFavoriteDetails(details)
	if err != nil {
		return err
	}
	if s.FavoriteDetails == nil {
		s.FavoriteDetails = make(map[string]FavoriteDetails)
	}
	key := FavoriteKey(room)
	previous, existed := s.FavoriteDetails[key]
	s.FavoriteDetails[key] = details
	if err := s.save(); err != nil {
		if existed {
			s.FavoriteDetails[key] = previous
		} else {
			delete(s.FavoriteDetails, key)
		}
		return err
	}
	return nil
}

func normalizeFavoriteDetails(details FavoriteDetails) (FavoriteDetails, error) {
	details.Alias, details.Group = strings.TrimSpace(details.Alias), strings.TrimSpace(details.Group)
	if utf8.RuneCountInString(details.Alias) > 48 || utf8.RuneCountInString(details.Group) > 24 {
		return details, errors.New("别名最多 48 字，分组最多 24 字")
	}
	return details, nil
}

func (s *State) FavoriteGroups() []string {
	groups := map[string]bool{}
	for _, room := range s.Favorites {
		if name := s.Details(room).Group; name != "" {
			groups[name] = true
		}
	}
	var result []string
	for name := range groups {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
