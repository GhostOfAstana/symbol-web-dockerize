"use strict";
// Форма работает без JavaScript; скрипт добавляет обратную связь и копирование.
const artForm = document.querySelector("#art-form");
artForm?.addEventListener("submit", () => {
    const button = document.querySelector("#generate");
    button.disabled = true;
    button.classList.add("loading");
    button.querySelector("span").textContent = "Создаём надпись…";
});
window.addEventListener("pageshow", () => {
    const button = document.querySelector("#generate");
    if (button) {
        button.disabled = false;
        button.classList.remove("loading");
        button.querySelector("span").textContent = "Создать надпись";
    }
});
document.querySelector("#copy-art")?.addEventListener("click", async () => {
    const status = document.querySelector("#copy-status");
    try {
        if (!navigator.clipboard)
            throw new Error("Буфер обмена недоступен. Выделите текст результата и скопируйте вручную.");
        await navigator.clipboard.writeText(document.querySelector("#art-output").textContent);
        status.textContent = "Результат скопирован.";
    }
    catch (error) {
        status.textContent = error.message || "Не удалось скопировать текст.";
    }
});
document.querySelector("#check-health")?.addEventListener("click", async (event) => {
    const button = event.currentTarget;
    const status = document.querySelector("#health-status");
    button.disabled = true;
    button.classList.add("loading");
    status.textContent = "Читаем показатели Docker…";
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 10000);
    try {
        const response = await fetch("/health-monitor", { signal: controller.signal, cache: "no-store" });
        const data = await response.json();
        if (!response.ok)
            throw new Error(data.error || "Монитор временно недоступен.");
        document.querySelector("#health-output").textContent = JSON.stringify(data, null, 2);
        status.textContent = "Показатели обновлены.";
    }
    catch (error) {
        status.textContent = error.name === "AbortError" ? "Показатели не получены за 10 секунд. Повторите попытку." : error.message;
    }
    finally {
        clearTimeout(timeout);
        button.disabled = false;
        button.classList.remove("loading");
    }
});
