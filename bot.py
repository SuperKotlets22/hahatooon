import telebot
from telebot import types
import requests
import os

# --- КОНФИГ ---
TOKEN = "8293823191:AAGqs7cDTFQfuvWoo6ulPTKoe1lsElgNSq0"

# Если есть переменная окружения (в Docker), берем её. Если нет — localhost.
GO_SERVER_URL = os.getenv("GO_SERVER_URL", "http://localhost:8080")

bot = telebot.TeleBot(TOKEN)

print("🐍 Python Bot v2.0 запущен...")

@bot.message_handler(commands=['start'])
def send_welcome(message):
    bot.reply_to(message, "👋 Привет! Я бот Т-Очереди.\n\nНапиши мне номер талона (например, A-105), чтобы управлять очередью.")

# Обработка текста (привязка талона)
@bot.message_handler(func=lambda message: True)
def handle_ticket(message):
    chat_id = message.chat.id
    ticket = message.text.strip().upper()

    if not ticket.startswith("A-"):
        bot.reply_to(message, "❌ Номер должен начинаться с A- (например, A-101)")
        return

    payload = {"ticket": ticket, "chat_id": chat_id}

    try:
        response = requests.post(f"{GO_SERVER_URL}/api/link_telegram", json=payload)
        
        if response.status_code == 200:
            # Создаем клавиатуру
            markup = types.InlineKeyboardMarkup()
            btn_pause = types.InlineKeyboardButton("⏯ Пауза / Вернуться", callback_data="pause")
            btn_leave = types.InlineKeyboardButton("❌ Покинуть очередь", callback_data="leave")
            markup.add(btn_pause, btn_leave)

            bot.reply_to(message, f"✅ Талон {ticket} привязан!\nТеперь ты можешь управлять очередью прямо отсюда.", reply_markup=markup)
        elif response.status_code == 404:
            bot.reply_to(message, "❌ Талон не найден. Займи очередь на сайте.")
        else:
            bot.reply_to(message, "⚠️ Ошибка сервера.")
            
    except Exception as e:
        print(e)

# Обработка кнопок
@bot.callback_query_handler(func=lambda call: True)
def callback_query(call):
    action = call.data
    chat_id = call.message.chat.id

    payload = {"chat_id": chat_id, "action": action}
    
    try:
        requests.post(f"{GO_SERVER_URL}/api/bot_action", json=payload)
        
        if action == "pause":
            bot.answer_callback_query(call.id, "Статус изменен!")
        elif action == "leave":
            bot.answer_callback_query(call.id, "Вы покинули очередь")
            bot.edit_message_text("Вы покинули очередь. До свидания!", chat_id, call.message.message_id)
            
    except Exception as e:
        print(e)

bot.infinity_polling()
