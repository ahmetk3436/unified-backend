package eracheck

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"strings"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"gorm.io/gorm"
)

type EraProfile struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Emoji       string `json:"emoji"`
	MusicTaste  string `json:"music_taste"`
	StyleTraits string `json:"style_traits"`
}

type Question struct {
	QuestionText string
	Category     string
	Options      [10]string
}

type QuizOption struct {
	Text string `json:"text"`
	Era  string `json:"era"`
}

var CategoryWeights = map[string]float64{
	"style": 1.5, "music": 1.2, "social": 1.0,
	"aesthetic": 1.3, "lifestyle": 0.8, "culture": 1.0,
}

func GetCategoryWeight(category string) float64 {
	if weight, ok := CategoryWeights[strings.ToLower(category)]; ok {
		return weight
	}
	return 1.0
}

var EraProfiles = map[string]EraProfile{
	"y2k":              {Key: "y2k", Title: "Y2K Baby", Description: "Low-rise jeans, butterfly clips, bedazzled everything, Paris Hilton energy", Color: "#FF69B4", Emoji: "💖", MusicTaste: "Britney Spears, Christina Aguilera, Destinys Child, NSYNC, early 2000s pop", StyleTraits: "Juicy Couture tracksuits, rhinestones, tiny bags, platform sandals, frosted lips"},
	"2016_tumblr":      {Key: "2016_tumblr", Title: "2016 Tumblr", Description: "Pastel hair, galaxy print, grunge aesthetic, indie music vibes", Color: "#FFB7B2", Emoji: "🦄", MusicTaste: "Arctic Monkeys, Lana Del Rey, The 1975, Halsey, Melanie Martinez", StyleTraits: "Chokers, flannel shirts, band tees, doc martens, flower crowns"},
	"2018_vsco":        {Key: "2018_vsco", Title: "VSCO Girl", Description: "Hydro flasks, scrunchies, 'sksksk', beachy aesthetic", Color: "#A8E6CF", Emoji: "📸", MusicTaste: "AJR, Khalid, Billie Eilish, LANY, Quinn XCII", StyleTraits: "Puka shell necklaces, oversized t-shirts, birkenstocks, scrunchies, metal straws"},
	"2020_cottagecore": {Key: "2020_cottagecore", Title: "Cottagecore", Description: "Baking bread, prairie dresses, nature, romanticizing rural life", Color: "#D4A373", Emoji: "🌻", MusicTaste: "Folk music, Taylor Swift (folklore/evermore), Bon Iver, Florence + The Machine", StyleTraits: "Floral dresses, straw hats, gardening, picnics, lace details"},
	"dark_academia":    {Key: "dark_academia", Title: "Dark Academia", Description: "Old libraries, poetry, vintage blazers, intellectual aesthetic", Color: "#3D2B1F", Emoji: "📚", MusicTaste: "Classical, Hozier, Mitski, Cigarettes After Sex, Tchaikovsky", StyleTraits: "Tweed blazers, turtlenecks, plaid skirts, oxford shoes, leather satchels"},
	"indie_sleaze":     {Key: "indie_sleaze", Title: "Indie Sleaze", Description: "Messy hair, smudged eyeliner, warehouse parties, effortlessly cool", Color: "#4A0E4E", Emoji: "🎸", MusicTaste: "The Strokes, Yeah Yeah Yeahs, LCD Soundsystem, Interpol, MGMT", StyleTraits: "Skinny jeans, leather jackets, American Apparel, wayfarers, messy hair"},
	"2022_clean_girl":  {Key: "2022_clean_girl", Title: "Clean Girl", Description: "Minimalist, slicked back hair, gold jewelry, expensive neutrals", Color: "#F5F5DC", Emoji: "✨", MusicTaste: "SZA, Dua Lipa, Beyoncé, Rihanna, Drake", StyleTraits: "Neutral tones, blazers, clean makeup, loafers, slicked buns"},
	"2024_mob_wife":    {Key: "2024_mob_wife", Title: "Mob Wife", Description: "Big fur coats, leopard print, luxury, bold confidence", Color: "#000000", Emoji: "💅", MusicTaste: "Frank Sinatra, Dean Martin, Adele, Lady Gaga", StyleTraits: "Leopard print, oversized sunglasses, leather, gold chains, fur coats"},
	"coastal_cowgirl":  {Key: "coastal_cowgirl", Title: "Coastal Cowgirl", Description: "Boots meet the beach, turquoise jewelry, sunset chaser vibes", Color: "#87CEEB", Emoji: "🤠", MusicTaste: "Kacey Musgraves, Shania Twain, Maren Morris, Orville Peck", StyleTraits: "Cowboy boots, denim cutoffs, turquoise jewelry, fringe details, woven bags"},
	"2025_demure":      {Key: "2025_demure", Title: "Very Demure", Description: "Mindful, cutesy, modest, polite and considerate", Color: "#E6E6FA", Emoji: "🎀", MusicTaste: "Soft pop, acoustic covers, Sabrina Carpenter, Chappell Roan", StyleTraits: "Bows, modest skirts, soft colors, cardigans, polite aesthetics"},
}

var EraKeys = []string{
	"y2k", "2016_tumblr", "2018_vsco", "2020_cottagecore", "dark_academia",
	"indie_sleaze", "2022_clean_girl", "2024_mob_wife", "coastal_cowgirl", "2025_demure",
}

func GetEraProfile(key string) (EraProfile, bool) {
	profile, exists := EraProfiles[key]
	return profile, exists
}

func GetRandomQuestion() Question {
	return SeedQuestions[rand.Intn(len(SeedQuestions))]
}

var SeedQuestions = []Question{
	{QuestionText: "Pick your go-to outfit for a casual day out:", Category: "style", Options: [10]string{"Low-rise jeans, baby tee, butterfly clips", "Black skinny jeans, band tee, combat boots", "Oversized tee, biker shorts, white sneakers", "Flowy floral dress with a straw hat", "Dark turtleneck, plaid skirt, loafers", "Skinny jeans, leather jacket, wayfarers", "Neutral matching set, gold jewelry, slicked bun", "Leopard print coat, oversized sunglasses, heels", "Cowboy boots, denim cutoffs, turquoise necklace", "Modest skirt, cardigan, bow in hair"}},
	{QuestionText: "What's your signature accessory?", Category: "style", Options: [10]string{"Butterfly clips and tinted sunglasses", "Stack of black rubber bracelets and studded belt", "Shell necklace and scrunchie on wrist", "Vintage locket and lace gloves", "Wire-rim glasses and leather satchel", "Wayfarers, messy eyeliner, and silver rings", "Simple gold hoops and dainty necklace", "Oversized shades and bold gold chains", "Turquoise jewelry and a fringe bag", "Satin bow and pearl stud earrings"}},
	{QuestionText: "How do you typically style your hair?", Category: "style", Options: [10]string{"Crimped or with butterfly clips and zigzag part", "Dip-dyed ends or ombré with side-swept bangs", "Beach waves with a messy braid", "Long braids with ribbons or flowers woven in", "Sleek low bun or straight with middle part", "Messy bedhead, maybe some product to look effortless", "Slicked-back bun or blowout with claw clip", "Big voluminous blowout with bold highlights", "Loose waves with a wide-brim hat", "Soft curls with a ribbon headband"}},
	{QuestionText: "What's on your favorite playlist?", Category: "music", Options: [10]string{"Britney Spears, NSYNC, Spice Girls", "Arctic Monkeys, Lana Del Rey, The 1975", "Jack Johnson, Vance Joy, acoustic vibes", "Fleetwood Mac, Taylor Swift folklore, indie folk", "Classical, Hozier, Mitski", "The Strokes, LCD Soundsystem, Yeah Yeah Yeahs", "SZA, The Weeknd, chill R&B", "Frank Sinatra, Dean Martin, Adele", "Kacey Musgraves, Shania Twain, country pop", "Soft pop, Sabrina Carpenter, Chappell Roan"}},
	{QuestionText: "Pick your ideal concert experience:", Category: "music", Options: [10]string{"Massive stadium with choreographed dance breaks", "Small indie venue, moody lighting, emotional lyrics", "Outdoor beach festival, barefoot in the sand", "Garden concert with string lights and blankets", "Intimate jazz club or poetry reading", "Sweaty warehouse show, raw garage rock energy", "VIP section, bottle service, sleek lounge", "Glamorous gala with live big band orchestra", "Outdoor rodeo with live country bands", "Cozy acoustic set in a candlelit café"}},
	{QuestionText: "It's Friday night. What are you doing?", Category: "social", Options: [10]string{"Mall hangout, then movie marathon sleepover", "Taking moody photos for Instagram, staying in", "Beach bonfire with friends, watching sunset", "Baking bread, reading by candlelight", "Art gallery opening or used bookstore browsing", "Warehouse party until sunrise", "Pilates class, then matcha with friends", "Dinner at the fanciest Italian restaurant in town", "Horseback riding into the sunset", "Journaling and a face mask with soft music"}},
	{QuestionText: "How would you describe your social media presence?", Category: "social", Options: [10]string{"Glitter edits, mirror selfies, emoji overload", "Dark aesthetic, deep quotes, black and white photos", "Sunset pics, nature shots, 'sksksk' in comments", "Cottage pics, flower arrangements, cozy vibes", "Moody bookshelf photos, poetry excerpts", "Concert photos, blurry flash shots, raw and real", "Minimalist grid, neutral tones, aesthetic flat lays", "Luxury lifestyle, designer unboxings, bold poses", "Golden hour selfies, boots on the dashboard", "Soft pastel feed, wholesome quotes, ribbon details"}},
	{QuestionText: "Describe your dream bedroom aesthetic:", Category: "aesthetic", Options: [10]string{"Inflatable furniture, bead curtain, CD player", "Fairy lights, band posters, dark bedding", "Polaroid wall, tapestry, plants everywhere", "Floral wallpaper, vintage furniture, dried flowers", "Dark walls, floor-to-ceiling bookshelves, vintage maps", "Bare brick walls, vintage records, messy desk", "All white, minimal decor, fresh flowers in a vase", "Velvet furniture, gold accents, crystal chandelier", "Rattan headboard, cactus plants, woven blankets", "Pastel pink walls, fluffy pillows, satin curtains"}},
	{QuestionText: "Which color palette speaks to your soul?", Category: "aesthetic", Options: [10]string{"Baby blue, bubblegum pink, lavender", "Black, deep burgundy, forest green", "Sandy beige, seafoam, sunset orange", "Cream, sage green, dusty rose", "Navy, burgundy, gold accents", "Black, red, silver with a gritty edge", "White, beige, taupe, soft brown", "Black, gold, deep red, leopard tones", "Sky blue, sandy tan, turquoise", "Lavender, soft pink, ivory"}},
	{QuestionText: "How do you edit your photos?", Category: "aesthetic", Options: [10]string{"Overexposed, soft focus, sparkles added", "High contrast, vignette, desaturated", "Warm filter, slight fade, natural light", "Soft, warm, slightly overexposed dreamy", "Moody, dark, film grain effect", "Flash on, no filter, raw nightlife energy", "Bright, clean, minimal editing", "High saturation, dramatic lighting, bold contrast", "Golden hour warmth, earthy tones, wide angle", "Soft pink tint, gentle brightness, pastel overlay"}},
	{QuestionText: "Pick your go-to drink order:", Category: "lifestyle", Options: [10]string{"Fruit smoothie or Capri Sun", "Black coffee, no sugar, at a dim café", "Iced coffee in a reusable metal straw cup", "Herbal tea in a vintage teacup", "Espresso martini or black tea", "Cheap beer at a dive bar", "Iced matcha latte with oat milk", "Negroni or an old fashioned, top shelf only", "Lemonade with a sprig of mint on the porch", "Pink strawberry frappuccino with whipped cream"}},
	{QuestionText: "Where's your dream vacation spot?", Category: "lifestyle", Options: [10]string{"Malibu or LA for the celebrity experience", "Moody London streets, record shops", "Beach town in California or Hawaii", "Countryside cottage in the English hills", "Prague or Vienna for architecture and cafés", "Brooklyn warehouse parties or Berlin techno clubs", "Tulum or Santorini for aesthetic resorts", "Amalfi Coast or Monaco, first class everything", "Montana ranch or beachside in the Carolinas", "Kyoto for cherry blossoms and tea ceremonies"}},
	{QuestionText: "What's your comfort movie or show?", Category: "culture", Options: [10]string{"Mean Girls, Clueless, Bring It On", "Perks of Being a Wallflower, Scott Pilgrim", "The Last Song, any beach movie", "Little Women, Pride and Prejudice", "Dead Poets Society, The Secret History vibes", "Almost Famous, The Runaways", "Emily in Paris, The Devil Wears Prada", "The Sopranos, Goodfellas, Scarface", "Yellowstone, A Star Is Born", "Bridgerton, Little Miss Sunshine"}},
	{QuestionText: "Who's your style icon?", Category: "culture", Options: [10]string{"Britney Spears, Christina Aguilera, Paris Hilton", "Lana Del Rey, Alexa Chung", "Gigi Hadid, any VS model off-duty", "Florence Pugh, Keira Knightley in period roles", "Timothée Chalamet, Zendaya", "Kate Moss, Alexa Chung, Pete Doherty", "Hailey Bieber, Kendall Jenner", "Jennifer Coolidge, Donatella Versace", "Kacey Musgraves, Shania Twain", "Sabrina Carpenter, Emma Chamberlain"}},
	{QuestionText: "Which phrase are you most likely to say?", Category: "culture", Options: [10]string{"That's hot, as if, whatever!", "I'm so done, literally can't even", "And I oop-, sksksk, save the turtles", "That's so wholesome, cozy vibes only", "How delightfully pretentious, I love it", "It's giving chaos and I'm here for it", "Living my best life, that's the aesthetic", "I'll make them an offer they can't refuse", "Yeehaw, let's ride into the sunset", "Very demure, very mindful, very cutesy"}},
}

var challengePrompts = []string{
	"Describe your dream outfit for a night out.",
	"What does your ideal workspace look like?",
	"If you could live in any decade, which one and why?",
	"Create a mood board description for your aesthetic.",
	"What's your signature accessory?",
	"Describe your go-to weekend look.",
	"If your vibe were a song, what would it be?",
	"What does your ideal brunch setup look like?",
	"Describe your dream vacation aesthetic.",
	"How would your friends describe your style in 3 words?",
}

// SeedQuizQuestions inserts quiz questions for a specific app.
func SeedQuizQuestions(db *gorm.DB, appID string) error {
	seeded := 0

	for _, sq := range SeedQuestions {
		var existing EraQuiz
		err := db.Scopes(tenant.ForTenant(appID)).Where("question = ?", sq.QuestionText).First(&existing).Error
		if err == nil {
			continue
		}

		var options []QuizOption
		for i, text := range sq.Options {
			if text == "" {
				continue
			}
			options = append(options, QuizOption{Text: text, Era: EraKeys[i]})
		}

		optionsJSON, err := json.Marshal(options)
		if err != nil {
			return err
		}

		category := sq.Category
		if category == "" {
			category = "general"
		}

		quiz := EraQuiz{
			AppID:    appID,
			Question: sq.QuestionText,
			Options:  optionsJSON,
			Category: category,
		}

		if err := db.Create(&quiz).Error; err != nil {
			return err
		}
		seeded++
	}

	if seeded > 0 {
		slog.Info("seeded quiz questions", "app_id", appID, "new", seeded, "total", len(SeedQuestions))
	}
	return nil
}
