package wouldyou

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Challenge represents a "Would You Rather" challenge.
type Challenge struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID     string         `gorm:"size:50;not null;index" json:"app_id"`
	OptionA   string         `gorm:"size:500;not null" json:"option_a"`
	OptionB   string         `gorm:"size:500;not null" json:"option_b"`
	Category  string         `gorm:"size:50" json:"category"`
	VotesA    int            `gorm:"default:0" json:"votes_a"`
	VotesB    int            `gorm:"default:0" json:"votes_b"`
	IsDaily   bool           `gorm:"default:false" json:"is_daily"`
	DailyDate time.Time      `gorm:"type:date" json:"daily_date"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Vote represents a user's vote on a challenge.
type Vote struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID       string         `gorm:"size:50;not null;index" json:"app_id"`
	UserID      uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	GuestID     string         `gorm:"size:255;index" json:"guest_id"`
	ChallengeID uuid.UUID      `gorm:"type:uuid;not null;index" json:"challenge_id"`
	Choice      string         `gorm:"size:1;not null" json:"choice"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// ChallengeStreak tracks user's voting streak.
type ChallengeStreak struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AppID         string         `gorm:"size:50;not null;index;uniqueIndex:idx_wy_streak_app_user" json:"app_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_wy_streak_app_user" json:"user_id"`
	CurrentStreak int            `gorm:"default:0" json:"current_streak"`
	LongestStreak int            `gorm:"default:0" json:"longest_streak"`
	TotalVotes    int            `gorm:"default:0" json:"total_votes"`
	LastVoteDate  time.Time      `gorm:"type:date" json:"last_vote_date"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// DailyChallenges - expanded pool of 60 challenges across 6 categories.
var DailyChallenges = []struct {
	OptionA  string
	OptionB  string
	Category string
}{
	// LIFE (10)
	{"Have unlimited money but no friends", "Have amazing friends but always be broke", "life"},
	{"Live in your dream house in a boring town", "Live in a tiny apartment in the best city", "life"},
	{"Always be 10 minutes late", "Always be 20 minutes early", "life"},
	{"Have a personal chef for life", "Have a personal driver for life", "life"},
	{"Never have to clean again", "Never have to cook again", "life"},
	{"Live without air conditioning", "Live without heating", "life"},
	{"Always have perfect weather", "Always find a parking spot", "life"},
	{"Be able to eat anything without gaining weight", "Need only 3 hours of sleep", "life"},
	{"Have a lifetime supply of your favorite food", "Have a lifetime supply of your favorite drink", "life"},
	{"Live in a world without music", "Live in a world without movies", "life"},
	// DEEP (10)
	{"Know when you'll die", "Know how you'll die", "deep"},
	{"Know the truth about every conspiracy", "Be happily ignorant about everything", "deep"},
	{"Relive the same day forever", "Jump to a random day every morning", "deep"},
	{"Know what everyone thinks of you", "Never know what anyone thinks of you", "deep"},
	{"Have the power to change the past", "Have the power to see the future", "deep"},
	{"Forget all your memories and start fresh", "Remember every single detail of your life", "deep"},
	{"Live a short exciting life", "Live a long boring life", "deep"},
	{"Be feared by everyone", "Be loved by everyone but never respected", "deep"},
	{"Know the answer to any one question", "Know the question to every answer", "deep"},
	{"Experience everything with double intensity", "Experience everything with half the emotion", "deep"},
	// SUPERPOWER (10)
	{"Be able to fly", "Be able to read minds", "superpower"},
	{"Be invisible whenever you want", "Be able to teleport anywhere", "superpower"},
	{"Have super strength", "Have super speed", "superpower"},
	{"Control time", "Control the weather", "superpower"},
	{"Talk to animals", "Speak every human language fluently", "superpower"},
	{"Have X-ray vision", "Have night vision", "superpower"},
	{"Breathe underwater", "Survive in outer space", "superpower"},
	{"Have photographic memory", "Be able to forget anything on command", "superpower"},
	{"Heal any wound instantly", "Never get sick again", "superpower"},
	{"Control fire", "Control water", "superpower"},
	// FUNNY (10)
	{"Have a permanent clown nose", "Have permanent clown shoes", "funny"},
	{"Accidentally send a text to your boss meant for your best friend", "Accidentally like a 3-year-old photo of your crush", "funny"},
	{"Only communicate in song lyrics", "Only communicate in movie quotes", "funny"},
	{"Sneeze glitter", "Burp confetti", "funny"},
	{"Have a laugh that sounds like a dolphin", "Have a sneeze that sounds like a foghorn", "funny"},
	{"Wear your clothes inside-out forever", "Wear your shoes on the wrong feet forever", "funny"},
	{"Have spaghetti for hair", "Sweat maple syrup", "funny"},
	{"Talk like a pirate forever", "Walk like a penguin forever", "funny"},
	{"Have to sing everything you say", "Have to dance everywhere you walk", "funny"},
	{"Have fingers as long as your legs", "Have legs as long as your fingers", "funny"},
	// LOVE (10)
	{"Find your soulmate but they live across the world", "Have a good relationship nearby but always wonder", "love"},
	{"Have a partner who is extremely funny", "Have a partner who is extremely smart", "love"},
	{"Know your partner's every thought", "Have your partner know your every thought", "love"},
	{"Have one true love that ends", "Have many good relationships that each fade", "love"},
	{"Fall in love at first sight", "Build love slowly over years", "love"},
	{"Date someone who is always honest", "Date someone who always tells you what you want to hear", "love"},
	{"Have a perfect wedding but rocky marriage", "Have an elopement but perfect marriage", "love"},
	{"Always agree with your partner", "Have passionate debates with your partner", "love"},
	{"Be with someone who remembers every detail", "Be with someone who lives in the moment", "love"},
	{"Have a love letter written about you", "Have a song written about you", "love"},
	// TECH (10)
	{"Never use social media again", "Never watch movies or TV again", "tech"},
	{"Have free WiFi everywhere", "Have free coffee everywhere", "tech"},
	{"Only use your phone 1 hour a day", "Never use a computer again", "tech"},
	{"Have the latest phone forever", "Have the fastest internet forever", "tech"},
	{"Live without GPS navigation", "Live without autocorrect", "tech"},
	{"Have a robot that cleans", "Have a robot that cooks", "tech"},
	{"Give up video games forever", "Give up streaming services forever", "tech"},
	{"Have your brain backed up to the cloud", "Have a chip that makes you 10x smarter", "tech"},
	{"Live in virtual reality full time", "Never be able to use VR at all", "tech"},
	{"Have AI write all your emails", "Have AI plan all your meals", "tech"},
}
