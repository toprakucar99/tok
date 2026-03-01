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

// --- VERİ YAPILARI (STRUCTS) ---
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

func main() {
	// 1. NEON DB BAĞLANTISI
	connStr := "postgresql://neondb_owner:npg_osBAh35rWzVC@ep-gentle-waterfall-alr3uyjz-pooler.c-3.eu-central-1.aws.neon.tech/neondb?sslmode=require"
	db, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatal("❌ Veritabanına bağlanılamadı: ", err)
	}
	defer db.Close(context.Background())
	fmt.Println("✅ Neon DB Bağlantısı Başarılı!")

	app := fiber.New()

	// 2. CORS AYARI
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH",
	}))

	// 3. STATİK DOSYALAR
	app.Static("/", "./static")

	// 4. SORULARI VE ŞIKLARI GETİR
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
			if err := rows.Scan(&q.ID, &q.Text, &optionsJSON); err != nil {
				continue
			}
			json.Unmarshal(optionsJSON, &q.Options)
			questions = append(questions, q)
		}
		return c.JSON(questions)
	})

	// 5. YARIŞMAYI BİTİR - DOĞRU CEVAP KONTROLLÜ TOKEN DAĞITIMI
	app.Post("/finish-survey", func(c *fiber.Ctx) error {
		var req FinishRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Geçersiz veri formatı"})
		}

		var userID int
		err := db.QueryRow(context.Background(), "SELECT id FROM users WHERE email=$1", req.Email).Scan(&userID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Kullanıcı bulunamadı"})
		}

		correctCount := 0
		tokenPerCorrect := 10 // Her doğru cevap başına verilecek token

		for _, ans := range req.Answers {
			// Önce cevabı DB'ye kaydet (Analiz için)
			_, _ = db.Exec(context.Background(),
				"INSERT INTO answers (user_id, question_id, option_id) VALUES ($1, $2, $3)",
				userID, ans.QuestionID, ans.OptionID)

			// Şık doğru mu kontrol et?
			var isCorrect bool
			err := db.QueryRow(context.Background(), "SELECT is_correct FROM options WHERE id=$1", ans.OptionID).Scan(&isCorrect)

			if err == nil && isCorrect {
				correctCount++
			}
		}

		// Toplam kazanılan token'ı hesapla
		totalEarned := correctCount * tokenPerCorrect

		// Kullanıcının token'larını güncelle
		if totalEarned > 0 {
			_, err = db.Exec(context.Background(), "UPDATE users SET tokens = tokens + $1 WHERE id = $2", totalEarned, userID)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Token güncellenemedi"})
			}
		}

		return c.JSON(fiber.Map{
			"success":       true,
			"correct_count": correctCount,
			"earned_tokens": totalEarned,
			"message":       fmt.Sprintf("%d doğru yaptınız ve %d Token kazandınız!", correctCount, totalEarned),
		})
	})

	// 6. GİRİŞ (LOGIN)
	app.Post("/login", func(c *fiber.Ctx) error {
		type LoginReq struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		var req LoginReq
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"success": false, "error": "Veri hatası"})
		}

		var dbPass string
		var currentTokens int
		err := db.QueryRow(context.Background(), "SELECT password, tokens FROM users WHERE email=$1", req.Email).Scan(&dbPass, &currentTokens)

		if err != nil || dbPass != req.Password {
			return c.Status(401).JSON(fiber.Map{"success": false, "error": "Hatalı giriş bilgileri"})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"tokens":  currentTokens,
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🚀 Server %s portunda uçuşa geçti!\n", port)
	log.Fatal(app.Listen(":" + port))
}
