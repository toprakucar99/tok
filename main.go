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
	// 1. NEON DB BAĞLANTISI (Kendi bağlantı stringini buraya yapıştır)
	connStr := "postgresql://neondb_owner:npg_osBAh35rWzVC@ep-gentle-waterfall-alr3uyjz-pooler.c-3.eu-central-1.aws.neon.tech/neondb?sslmode=require"
	db, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		log.Fatal("❌ Veritabanına bağlanılamadı: ", err)
	}
	defer db.Close(context.Background())
	fmt.Println("✅ Neon DB Bağlantısı Başarılı!")

	app := fiber.New()

	// 2. CORS AYARI (Frontend'in bağlanabilmesi için)
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH",
	}))

	// 3. STATİK DOSYALAR (index.html'in olduğu klasör)
	app.Static("/", "./static")

	// 4. SORULARI VE ŞIKLARI GETİR (Çoktan Seçmeli Yapı)
	app.Get("/questions", func(c *fiber.Ctx) error {
		// Postgres'in JSON yeteneklerini kullanarak soruları ve şıkları tek seferde çekiyoruz
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
			return c.Status(500).JSON(fiber.Map{"error": "Veritabanı hatası: " + err.Error()})
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

	// 5. ANKETİ BİTİR VE CEVAPLARI KAYDET
	app.Post("/finish-survey", func(c *fiber.Ctx) error {
		var req FinishRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Geçersiz veri formatı"})
		}

		// Kullanıcı ID'sini bul
		var userID int
		err := db.QueryRow(context.Background(), "SELECT id FROM users WHERE email=$1", req.Email).Scan(&userID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Kullanıcı bulunamadı"})
		}

		// Cevapları 'answers' tablosuna tek tek ekle
		for _, ans := range req.Answers {
			_, err = db.Exec(context.Background(),
				"INSERT INTO answers (user_id, question_id, option_id) VALUES ($1, $2, $3)",
				userID, ans.QuestionID, ans.OptionID)
			if err != nil {
				fmt.Println("Cevap kaydedilemedi:", err)
			}
		}

		// Kullanıcıya 10 Token ödül ver
		_, err = db.Exec(context.Background(), "UPDATE users SET tokens = tokens + 10 WHERE id = $1", userID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Token güncellenemedi"})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Cevaplar kaydedildi ve 10 Token hesabınıza tanımlandı!",
		})
	})

	// 6. GİRİŞ (LOGIN) - Basit versiyon
	app.Post("/login", func(c *fiber.Ctx) error {
		type LoginReq struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		var req LoginReq
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Hata"})
		}

		var dbPass string
		err := db.QueryRow(context.Background(), "SELECT password FROM users WHERE email=$1", req.Email).Scan(&dbPass)
		if err != nil || dbPass != req.Password {
			return c.Status(401).JSON(fiber.Map{"error": "Hatalı giriş"})
		}

		return c.JSON(fiber.Map{"success": true, "token": "dummy-token"})
	})

	// Port Ayarı
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🚀 Server %s portunda uçuşa geçti!\n", port)
	log.Fatal(app.Listen(":" + port))
}
