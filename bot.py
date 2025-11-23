import os
import telebot
import requests

TOKEN = "8293823191:AAGqs7cDTFQfuvWoo6ulPTKoe1lsElgNSq0" 

GO_SERVER_URL = os.getenv("GO_SERVER_URL", "http://backend:8080")

bot = telebot.TeleBot(TOKEN)

print("🐍 Python Bot (Lite) запущен...")

@bot.message_handler(commands=['start'])
def send_welcome(message):
    bot.reply_to(message, "👋 Привет! Я бот Т-Очереди.\n\nПросто напиши мне номер талона (например, A-105), и я пришлю уведомление, когда подойдет твоя очередь!")

@bot.message_handler(func=lambda message: True)
def handle_ticket(message):
    chat_id = message.chat.id
    ticket = message.text.strip().upper()

    if not ticket.startswith("A-"):
        bot.reply_to(message, "❌ Номер должен начинаться с A- (например, A-101)")
        return

    payload = {
        "ticket": ticket,
        "chat_id": chat_id
    }

    try:
        response = requests.post(f"{GO_SERVER_URL}/api/link_telegram", json=payload)
        
        if response.status_code == 200:
            bot.reply_to(message, f"✅ Талон {ticket} успешно привязан!\n\nЯ напишу, когда нужно будет подходить к стойке. Можешь сворачивать Telegram.")
            
        elif response.status_code == 404:
            bot.reply_to(message, "❌ Талон не найден в активной очереди.\nПроверь номер или получи новый на сайте.")
        else:
            bot.reply_to(message, "⚠️ Ошибка сервера. Попробуй позже.")
            
    except Exception as e:
        print(f"Error: {e}")
        bot.reply_to(message, "🔌 Не могу связаться с сервером очереди.")

if __name__ == "__main__":
    bot.infinity_polling()