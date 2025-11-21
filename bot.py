import telebot
import requests
import json

# --- КОНФИГ ---
TOKEN = "8293823191:AAGqs7cDTFQfuvWoo6ulPTKoe1lsElgNSq0"
GO_SERVER_URL = "http://localhost:8080/api/link_telegram" # Куда стучаться в Go

bot = telebot.TeleBot(TOKEN)

print("🐍 Python Bot запущен...")

@bot.message_handler(commands=['start'])
def send_welcome(message):
    bot.reply_to(message, "👋 Привет! Я бот Т-Очереди.\n\nНапиши мне свой номер талона (например, A-105), и я позову тебя, когда подойдет время!")

@bot.message_handler(func=lambda message: True)
def handle_ticket(message):
    chat_id = message.chat.id
    ticket = message.text.strip().upper() # Делаем A-105 из a-105

    # Проверка формата (простая)
    if not ticket.startswith("A-"):
        bot.reply_to(message, "❌ Непохоже на талон. Номер должен начинаться с A- (например, A-101)")
        return

    # Отправляем данные в Go Backend
    payload = {
        "ticket": ticket,
        "chat_id": chat_id
    }

    try:
        response = requests.post(GO_SERVER_URL, json=payload)
        
        if response.status_code == 200:
            bot.reply_to(message, f"✅ Супер! Талон {ticket} привязан.\nЖди уведомления!")
        elif response.status_code == 404:
            bot.reply_to(message, "❌ Такой талон не найден в очереди. Проверь номер.")
        else:
            bot.reply_to(message, "⚠️ Ошибка на сервере. Попробуй позже.")
            
    except Exception as e:
        print(e)
        bot.reply_to(message, "🔌 Не могу связаться с сервером очереди.")

# Запуск (polling)
bot.infinity_polling()