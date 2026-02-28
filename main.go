package main

import (
	"context"
	"fmt"
	"log"
	"os" // Port için gerekli

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/jackc/pgx/v4"
)

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
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))

	// --- KRİTİK EKLEME: FRONTEND DOSYALARI ---
	// "static" klasöründeki dosyaları ana dizinde sunar
	app.Static("/", "./static")
	// ----------------------------------------

	// 3. GİRİŞ (LOGIN) ROTASI
	app.Post("/login", func(c *fiber.Ctx) error {
		type LoginRequest struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		var req LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Geçersiz istek"})
		}

		var dbPassword string
		err := db.QueryRow(context.Background(), "SELECT password FROM users WHERE email=$1", req.Email).Scan(&dbPassword)

		if err != nil || dbPassword != req.Password {
			return c.Status(401).JSON(fiber.Map{"error": "E-posta veya şifre hatalı!"})
		}

		return c.JSON(fiber.Map{"token": "gizli-anahtar-123", "success": true})
	})

	// 4. SORULARI GETİRME ROTASI
	app.Get("/questions", func(c *fiber.Ctx) error {
		rows, err := db.Query(context.Background(), "SELECT id, text FROM questions")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Sorular çekilemedi"})
		}
		defer rows.Close()

		var questions []fiber.Map
		for rows.Next() {
			var id int
			var text string
			if err := rows.Scan(&id, &text); err == nil {
				questions = append(questions, fiber.Map{"id": id, "text": text})
			}
		}

		if len(questions) == 0 {
			return c.JSON([]fiber.Map{{"id": 1, "text": "Henüz anket sorusu yok!"}})
		}
		return c.JSON(questions)
	})

	// 5. ANKET BİTİRME VE TOKEN KAZANMA ROTASI
	app.Post("/finish-survey", func(c *fiber.Ctx) error {
		type FinishRequest struct {
			Email string `json:"email"`
		}
		var req FinishRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Hata"})
		}

		_, err := db.Exec(context.Background(), "UPDATE users SET tokens = tokens + 10 WHERE email = $1", req.Email)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Güncellenemedi"})
		}

		var totalTokens int
		db.QueryRow(context.Background(), "SELECT tokens FROM users WHERE email=$1", req.Email).Scan(&totalTokens)

		return c.JSON(fiber.Map{
			"success":      true,
			"total_tokens": totalTokens,
		})
	})

	// --- KRİTİK EKLEME: SAYFA YENİLEME DESTEĞİ ---
	// Kullanıcı Fly.io linkinde dolaşırken 404 almasın diye
	app.Get("/*", func(c *fiber.Ctx) error {
		return c.SendFile("./static/index.html")
	})
	// ---------------------------------------------

	// PORT AYARI (Fly.io için dinamik port)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("🚀 Backend %s portunda ateşlendi!\n", port)
	log.Fatal(app.Listen(":" + port))
}
