package rules

import (
	"regexp"
	"strings"
)

func IsGenericMediaDir(name string) bool {
	_, ok := GenericMediaDirNames[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func ParseSeasonDirNumber(name string) *int {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return nil
	}
	for _, item := range seasonDirPatterns {
		if m := item.re.FindStringSubmatch(raw); len(m) >= 2 {
			if n := item.extract(m); n != nil {
				return n
			}
		}
	}
	return nil
}

func IsSeasonDirName(name string) bool {
	return ParseSeasonDirNumber(name) != nil
}

var explicitSeasonTokenRe = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:s\d{1,3}e\d{1,4}|\d{1,3}\s*x\s*\d{1,4}|season\s*\d{1,3})|第\s*(?:\d{1,3}|[零〇一二两三四五六七八九十百]+)\s*季`)

// 显式季号优先于解析默认 Season=1
func HasExplicitSeasonToken(name string) bool {
	return explicitSeasonTokenRe.MatchString(name)
}

func LooksLikeTVFile(parsed ParsedMedia, ancestors []Ancestor) RuleResult {
	return LooksLikeTVFileWithName(parsed, ancestors, "")
}

func LooksLikeTVFileWithName(parsed ParsedMedia, ancestors []Ancestor, fileName string) RuleResult {
	reasons := make([]string, 0, 4)
	score := 0.0
	if parsed.Season != nil && parsed.Episode != nil {
		if fileName == "" || HasExplicitSeasonToken(fileName) || hasTVHintAncestor(ancestors) {
			reasons = append(reasons, "文件名匹配 S/E 模式")
			score += 0.7
		}
	} else if parsed.Episode != nil && hasSpecialContentAncestor(ancestors) {
		reasons = append(reasons, "文件名含集数且位于番外/特别篇目录")
		score += 0.7
	}
	for _, anc := range ancestors {
		if IsSeasonDirName(anc.Name) {
			reasons = append(reasons, "祖先目录是季目录: "+anc.Name)
			score += 0.5
			break
		}
	}
	for _, anc := range ancestors {
		if isStructuralSpecialDirName(anc.Name) {
			reasons = append(reasons, "祖先目录是番外/特别篇: "+anc.Name)
			score += 0.5
			break
		}
	}
	if score >= 0.5 {
		if score > 1 {
			score = 1
		}
		return RuleResult{Matched: true, Score: score, Reasons: reasons}
	}
	return RuleResult{Matched: false, Score: score, Reasons: reasons}
}

func PickTVShowInfo(ancestors []Ancestor, fileParsed ParsedMedia) (showDirID, showDirName string, parsed ParsedMedia) {
	for idx := len(ancestors) - 1; idx >= 0; idx-- {
		dir := ancestors[idx]
		if IsGenericMediaDir(dir.Name) || IsSeasonDirName(dir.Name) || IsEpisodeRangeDirName(dir.Name) ||
			isCollectionContainerDir(dir.Name, nil) || isStructuralSpecialDirName(dir.Name) {
			continue
		}
		if looksLikeStandaloneMovieDir(dir.Name) {
			if showID, _, _ := PickTVShowInfo(ancestors[:idx], ParsedMedia{Season: intPtr(1), Episode: intPtr(1), Type: "episode"}); showID != "" {
				continue
			}
		}
		dirParsed := NormalizeParsedMedia(ParseDirName(dir.Name))
		if dirParsed.Title != "" {
			return dir.ID, dir.Name, dirParsed
		}
	}
	title := strings.TrimSpace(fileParsed.Title)
	return "", "", ParsedMedia{
		Title:   title,
		Year:    fileParsed.Year,
		Season:  fileParsed.Season,
		Episode: fileParsed.Episode,
		Type:    "episode",
	}
}

func hasSpecialContentAncestor(ancestors []Ancestor) bool {
	for _, anc := range ancestors {
		if isStructuralSpecialDirName(anc.Name) {
			return true
		}
	}
	return false
}

func hasTVHintAncestor(ancestors []Ancestor) bool {
	for _, anc := range ancestors {
		if IsSeasonDirName(anc.Name) || IsEpisodeRangeDirName(anc.Name) || isStructuralSpecialDirName(anc.Name) {
			return true
		}
		parsed := NormalizeParsedMedia(ParseDirName(anc.Name))
		if parsed.Season != nil {
			return true
		}
	}
	return false
}

func isSpecialContentDirName(name string) bool {
	if IsSeasonDirName(name) {
		return false
	}
	return specialContentDirRe.MatchString(name)
}

func isStructuralSpecialDirName(name string) bool {
	return isSpecialContentDirName(name) && !looksLikeStandaloneMovieDir(name)
}

func isCollectionContainerDir(name string, childDirNames []string) bool {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return false
	}
	if LooksLikeSceneMovieRelease(raw) {
		return false
	}
	dirParsed := NormalizeParsedMedia(ParseDirName(raw))
	title := strings.TrimSpace(dirParsed.Title)
	if title != "" && ScoreTitleForTMDB(title) >= 0.45 {
		if !collectionContainerStrongHintRe.MatchString(raw) {
			return false
		}
	}
	if collectionContainerHintRe.MatchString(raw) {
		return true
	}
	if len(childDirNames) > 0 {
		seasonCount := 0
		for _, child := range childDirNames {
			if IsSeasonDirName(child) {
				seasonCount++
			}
		}
		if seasonCount >= 2 {
			return true
		}
	}
	if seasonRangeTitleRe.MatchString(title) {
		return true
	}
	return false
}

func looksLikeStandaloneMovieDir(name string) bool {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return false
	}
	if IsGenericMediaDir(raw) || IsSeasonDirName(raw) || IsEpisodeRangeDirName(raw) ||
		isCollectionContainerDir(raw, nil) {
		return false
	}
	dirParsed := NormalizeParsedMedia(ParseDirName(raw))
	title := strings.TrimSpace(dirParsed.Title)
	if title == "" {
		return false
	}
	if isSpecialContentDirName(raw) {
		id := FindTMDBIDInName(raw)
		if dirParsed.Year == nil && id == "" {
			return false
		}
		remainder := strings.TrimSpace(specialContentDirRe.ReplaceAllString(title, " "))
		if remainder == "" && id == "" {
			return false
		}
		return ScoreTitleForTMDB(title) >= 0.45
	}
	if dirParsed.Year == nil {
		return false
	}
	if seasonOnlyTitleRe.MatchString(title) {
		return false
	}
	if standaloneMovieDirHintRe.MatchString(raw) {
		return true
	}
	return ScoreTitleForTMDB(title) >= 0.45
}

func IsStandaloneMovieDirName(name string) bool {
	return looksLikeStandaloneMovieDir(name)
}

var (
	specialContentDirRe             = regexpMust(`(?:^|[\s._\-（(【\[])(?:番外篇?|特别篇|特別篇|前传|后传|外传|OVA|OAD|SP|Side Story|Specials?)(?:[\s._\-）)】\]\']|$)`)
	collectionContainerHintRe       = regexpMust(`(?i)(?:\+|＋|/|(?:前?第?[一二三四五六七八九十\d]+季[与和]|[与和]前?第?[一二三四五六七八九十\d]+季|季[与和][前第]?[一二三四五六七八九十\d]+)|打包|合集|全集|全季|各季|前几季|前五季|前\d+季|番外.*剧场|剧场.*番外|番外\+|\+番外|季\+|\+季|多季|seasons?\s*[\+\&]|extras?\s*[\+\&])`)
	collectionContainerStrongHintRe = collectionContainerHintRe
	seasonRangeTitleRe              = regexpMust(`^前?[一二三四五六七八九十\d]+季$`)
	standaloneMovieDirHintRe        = regexpMust(`(?i)(?:剧场版|映画|电影版|大电影|院线版|Movie\s*Edition)`)
	seasonOnlyTitleRe               = regexpMust(`(?i)^第\s*\d{1,3}\s*季$`)
)

func regexpMust(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
