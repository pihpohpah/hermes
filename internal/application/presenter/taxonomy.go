package presenter

// Таксономия продукта для серверного рендера: сценарии, категории, типы
// меток, способы добраться и определения фильтров. Зеркалит taxonomy.js макета.

type Scenario struct {
	ID          string
	Label       string
	Icon        string
	Description string
}

var Scenarios = []Scenario{
	{ID: "city", Label: "Город", Icon: "house", Description: "Улицы, музеи, сцены и андеграунд"},
	{ID: "nature", Label: "Природа", Icon: "tree-pine", Description: "Сопки, водопады, озёра и тропы"},
	{ID: "sea", Label: "Море", Icon: "shell", Description: "Бухты, пляжи, маяки и острова"},
}

type Category struct {
	ID    string
	Label string
	Icon  string
}

var Categories = map[string][]Category{
	"city": {
		{ID: "walks", Label: "Прогулки", Icon: "footprints"},
		{ID: "viewpoints", Label: "Смотровые", Icon: "binoculars"},
		{ID: "history", Label: "История", Icon: "castle"},
		{ID: "museums", Label: "Музеи", Icon: "landmark"},
		{ID: "art", Label: "Арт-пространства", Icon: "palette"},
		{ID: "stage", Label: "Музыка и сцена", Icon: "music"},
	},
	"nature": {
		{ID: "hiking", Label: "Сопки и тропы", Icon: "mountain"},
		{ID: "waterfalls", Label: "Водопады", Icon: "droplets"},
		{ID: "lakes", Label: "Озёра", Icon: "fish"},
		{ID: "parks", Label: "Парки и сады", Icon: "flower-2"},
		{ID: "capes", Label: "Мысы и скалы", Icon: "compass"},
		{ID: "reserves", Label: "Заповедные места", Icon: "leaf"},
	},
	"sea": {
		{ID: "beaches", Label: "Пляжи", Icon: "sun"},
		{ID: "bays", Label: "Бухты", Icon: "sailboat"},
		{ID: "lighthouses", Label: "Маяки", Icon: "lighthouse"},
		{ID: "islands", Label: "Острова", Icon: "shell"},
		{ID: "diving", Label: "Дайвинг", Icon: "anchor"},
		{ID: "boats", Label: "Морские прогулки", Icon: "ship"},
	},
}

var EventCategories = map[string]Category{
	"concert":    {ID: "concert", Label: "Концерт", Icon: "music"},
	"exhibition": {ID: "exhibition", Label: "Выставка", Icon: "palette"},
	"tour":       {ID: "tour", Label: "Экскурсия", Icon: "footprints"},
	"festival":   {ID: "festival", Label: "Фестиваль", Icon: "party-popper"},
	"sport":      {ID: "sport", Label: "Спорт", Icon: "trophy"},
	"party":      {ID: "party", Label: "Андеграунд", Icon: "mic-vocal"},
	"lecture":    {ID: "lecture", Label: "Лекция", Icon: "presentation"},
	"market":     {ID: "market", Label: "Маркет", Icon: "shopping-bag"},
	// Категории импорта event-agent (webscraper), миграция 002.
	"theater":    {ID: "theater", Label: "Театр", Icon: "ticket"},
	"workshop":   {ID: "workshop", Label: "Мастер-класс", Icon: "hammer"},
	"meetup":     {ID: "meetup", Label: "Митап", Icon: "users"},
	"conference": {ID: "conference", Label: "Конференция", Icon: "presentation"},
	"standup":    {ID: "standup", Label: "Стендап", Icon: "mic-vocal"},
	"quiz":       {ID: "quiz", Label: "Квиз", Icon: "brain"},
	"kids":       {ID: "kids", Label: "Детям", Icon: "baby"},
	"film":       {ID: "film", Label: "Кино", Icon: "clapperboard"},
	"other":      {ID: "other", Label: "Разное", Icon: "map-pin"},
}

type PinKind struct {
	ID      string
	Label   string
	Icon    string
	Example string
}

var PinKinds = []PinKind{
	{ID: "sport", Label: "Спорт", Icon: "volleyball", Example: "Футбол 5 на 5, нужно ещё двое"},
	{ID: "walk", Label: "Прогулка", Icon: "footprints", Example: "Идём на закат к маяку"},
	{ID: "music", Label: "Музыка", Icon: "guitar", Example: "Джем на Фокина, берите инструменты"},
	{ID: "games", Label: "Игры", Icon: "gamepad-2", Example: "Настолки в кофейне, есть свободные места"},
	{ID: "help", Label: "Помощь", Icon: "hand-helping", Example: "Нужно донести диван до 3 этажа"},
	{ID: "other", Label: "Другое", Icon: "message-circle", Example: "Кто со мной на фотовыставку?"},
}

var AccessModes = map[string]Category{
	"walk":    {ID: "walk", Label: "Пешком", Icon: "footprints"},
	"transit": {ID: "transit", Label: "Общественный транспорт", Icon: "bus"},
	"car":     {ID: "car", Label: "На машине", Icon: "car"},
	"ferry":   {ID: "ferry", Label: "Паром", Icon: "ship"},
	"boat":    {ID: "boat", Label: "Катер или лодка", Icon: "sailboat"},
}

var DifficultyLabels = map[string]string{"easy": "Лёгкая", "medium": "Средняя", "hard": "Сложная"}
var BeachLabels = map[string]string{"sand": "Песок", "pebble": "Галька", "rocks": "Скалы", "none": "Нет пляжа"}

// FilterOption — один вариант в фильтре-чипе.
type FilterOption struct {
	Value string
	Label string
}

// FilterSpec описывает одно поле фильтров сценария.
type FilterSpec struct {
	ID      string
	Type    string // toggle | single | multi
	Label   string
	Hint    string
	Options []FilterOption
}

var distanceOptions = []FilterOption{
	{Value: "", Label: "Любое"},
	{Value: "30", Label: "До 30 км"},
	{Value: "100", Label: "До 100 км"},
	{Value: "250", Label: "До 250 км"},
}

func difficultyOptions() []FilterOption {
	out := make([]FilterOption, 0, len(DifficultyLabels))
	for _, code := range []string{"easy", "medium", "hard"} {
		out = append(out, FilterOption{Value: code, Label: DifficultyLabels[code]})
	}

	return out
}

var Filters = map[string][]FilterSpec{
	"city": {
		{ID: "free", Type: "toggle", Label: "Бесплатно"},
		{ID: "walkable", Type: "toggle", Label: "Пешком от центра", Hint: "Не дальше 2,5 км от Центральной площади"},
		{ID: "kids", Type: "toggle", Label: "Можно с детьми"},
	},
	"nature": {
		{ID: "distance", Type: "single", Label: "Расстояние от центра", Options: distanceOptions},
		{ID: "difficulty", Type: "multi", Label: "Сложность", Options: difficultyOptions()},
		{ID: "noCar", Type: "toggle", Label: "Можно без машины", Hint: "Автобус, электричка или паром"},
		{ID: "free", Type: "toggle", Label: "Бесплатно"},
	},
	"sea": {
		{ID: "beach", Type: "multi", Label: "Берег", Options: []FilterOption{
			{Value: "sand", Label: "Песок"},
			{Value: "pebble", Label: "Галька"},
			{Value: "rocks", Label: "Скалы"},
		}},
		{ID: "distance", Type: "single", Label: "Расстояние от центра", Options: distanceOptions},
		{ID: "swim", Type: "toggle", Label: "Можно купаться", Hint: "Есть купальный сезон"},
		{ID: "noCar", Type: "toggle", Label: "Можно без машины", Hint: "Автобус, электричка или паром"},
	},
}

var Sorts = []FilterOption{
	{Value: "recommended", Label: "Сначала интересные"},
	{Value: "distance", Label: "Ближе к центру"},
	{Value: "title", Label: "По алфавиту"},
}

// CategoryInfo резолвит подпись и иконку категории объекта.
func CategoryInfo(scenario, category, itemType string) Category {
	if itemType == "event" {
		if info, ok := EventCategories[category]; ok {
			return info
		}

		return Category{ID: category, Label: category, Icon: "map-pin"}
	}
	if info, ok := Categories[scenario]; ok {
		for _, candidate := range info {
			if candidate.ID == category {
				return candidate
			}
		}
	}

	return Category{ID: category, Label: category, Icon: "map-pin"}
}

// PinKindInfo резолвит тип быстрой метки.
func PinKindInfo(kind string) PinKind {
	for _, candidate := range PinKinds {
		if candidate.ID == kind {
			return candidate
		}
	}

	return PinKinds[len(PinKinds)-1]
}

// ScenarioInfo резолвит сценарий.
func ScenarioInfo(id string) Scenario {
	for _, candidate := range Scenarios {
		if candidate.ID == id {
			return candidate
		}
	}

	return Scenarios[0]
}
