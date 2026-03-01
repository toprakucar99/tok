package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jackc/pgx/v4"
)

// --- VERİ YAPILARI ---
type Option struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

type Question struct {
	ID      int      `json:"id"`
	Text    string   `json:"text"`
	Options []Option `json:"options"`
}

type AnswerReq struct {
	QuestionID int `json:"question_id"`
	OptionID   int `json:"option_id"`
}

type FinishRequest struct {
	Email   string      `json:"email"`
	Answers []AnswerReq `json:"answers"`
}

// Analiz için yeni yapı
type QuestionResult struct {
	QuestionID     int    `json:"question_id"`
	IsCorrect      bool   `json:"is_correct"`
	UserOptText    string `json:"user_option_text"`
	CorrectOptText string `json:"correct_option_text"`
}

func main() {
	connStr := "postgresql://neondb_owner:npg_osBAh35rWzVC@ep-gentle-waterfall-alr3uyjz-pooler.c-3.eu-central-1.aws.neon.tech/neondb?sslmode=require"
	db, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatal("❌ Veritabanına bağlanılamadı: ", err)
	}
	defer db.Close(context.Background())

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH",
	}))

	app.Static("/", "./static")

	app.Get("/questions", func(c *fiber.Ctx) error {
		query := `
            SELECT q.id, q.text, 
                   COALESCE(json_agg(json_build_object('id', o.id, 'text', o.option_text) ORDER BY o.id) FILTER (WHERE o.id IS NOT NULL), '[]') as options
            FROM questions q
            LEFT JOIN options o ON q.id = o.question_id
            GROUP BY q.id, q.text
            ORDER BY q.id
        `
		rows, err := db.Query(context.Background(), query)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Veritabanı hatası"})
		}
		defer rows.Close()

		var questions []Question
		for rows.Next() {
			var q Question
			var optionsJSON []byte
			rows.Scan(&q.ID, &q.Text, &optionsJSON)
			json.Unmarshal(optionsJSON, &q.Options)
			questions = append(questions, q)
		}
		return c.JSON(questions)
	})

	// --- GÜNCELLENEN ANALİZLİ FİNİSH ENDPOİNTİ ---
	app.Post("/finish-survey", func(c *fiber.Ctx) error {
		var req FinishRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Geçersiz veri"})
		}

		var userID int
		db.QueryRow(context.Background(), "SELECT id FROM users WHERE email=$1", req.Email).Scan(&userID)

		var results []QuestionResult
		correctCount := 0
		tokenPerCorrect := 5 // Kanka senin istediğin: Doğru başı 5 token

		for _, ans := range req.Answers {
			var isCorrect bool
			var userText string
			var correctText string

			// 1. Kullanıcının seçtiği şıkkın metni ve doğruluğunu al
			db.QueryRow(context.Background(),
				"SELECT option_text, is_correct FROM options WHERE id=$1",
				ans.OptionID).Scan(&userText, &isCorrect)

			// 2. O sorunun asıl doğru cevabını bul (Analiz ekranı için)
			db.QueryRow(context.Background(),
				"SELECT option_text FROM options WHERE question_id=$1 AND is_correct=TRUE",
				ans.QuestionID).Scan(&correctText)

			if isCorrect {
				correctCount++
			}

			// Listeye ekle
			results = append(results, QuestionResult{
				QuestionID:     ans.QuestionID,
				IsCorrect:      isCorrect,
				UserOptText:    userText,
				CorrectOptText: correctText,
			})

			// Veritabanına cevabı kaydet
			db.Exec(context.Background(), "INSERT INTO answers (user_id, question_id, option_id) VALUES ($1, $2, $3)", userID, ans.QuestionID, ans.OptionID)
		}

		totalEarned := correctCount * tokenPerCorrect
		db.Exec(context.Background(), "UPDATE users SET tokens = tokens + $1 WHERE id = $2", totalEarned, userID)

		return c.JSON(fiber.Map{
			"success":       true,
			"results":       results, // Analiz listesi burda
			"correct_count": correctCount,
			"earned_tokens": totalEarned,
			"message":       fmt.Sprintf("%d doğru ile %d Token kazandınız!", correctCount, totalEarned),
		})
	})

	app.Post("/login", func(c *fiber.Ctx) error {
		type LoginReq struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		var req LoginReq
		c.BodyParser(&req)
		var dbPass string
		var currentTokens int
		err := db.QueryRow(context.Background(), "SELECT password, tokens FROM users WHERE email=$1", req.Email).Scan(&dbPass, &currentTokens)
		if err != nil || dbPass != req.Password {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Hatalı giriş"})
		}
		return c.JSON(fiber.Map{"success": true, "tokens": currentTokens})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))
}
