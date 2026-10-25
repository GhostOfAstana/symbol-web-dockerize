"use strict";
// Используйте уже доступные Playwright и браузер; скрипт ничего не устанавливает.
const assert = require("node:assert/strict"), path = require("node:path");
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || process.argv[4] || "playwright");
(async () => {
    const executable = process.env.BROWSER_EXECUTABLE || process.argv[5], browser = await chromium.launch({ headless: true, ...(executable ? { executablePath: executable } : {}) });
    try {
        const page = await browser.newPage(), errors = [];
        page.on("pageerror", e => errors.push(e.message));
        const base = process.argv[2] || "http://localhost:8080";
        await page.goto(base, { waitUntil: "networkidle" });
        for (const width of [375, 768, 1920]) {
            await page.setViewportSize({ width, height: 1000 });
            assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `переполнение при ${width}`);
            if (process.argv[3])
                await page.screenshot({ path: path.join(process.argv[3], `dockerize-${width}.png`), fullPage: true });
        }
        await page.setViewportSize({ width: 1280, height: 1000 });
        await page.locator("#text-input").fill("WELCOME");
        await page.locator("#recommend-banner").click();
        await page.waitForFunction(() => !document.querySelector("#recommend-banner").disabled);
        assert.ok(["standard", "shadow", "thinkertoy"].includes(await page.locator("#banner").inputValue()));
        await page.locator("#variations").click();
        await page.locator("#suggestions button").first().waitFor();
        assert.ok(await page.locator("#suggestions button").count() >= 3);
        await page.locator("#suggestions button").first().click();
        for (const banner of ["standard", "shadow", "thinkertoy"]) {
            await page.locator("#text-input").fill("hello");
            await page.locator("#banner").selectOption(banner);
            await page.locator("#generate").click();
            assert.ok((await page.locator("#art-output").textContent()).length > 30);
        }
        await page.locator("#check-health").click();
        await page.waitForFunction(() => !document.querySelector("#check-health").disabled);
        assert.ok((await page.locator("#health-status").textContent()).length > 0);
        assert.equal((await page.request.get(base + "/healthz")).status(), 200);
        assert.deepEqual(errors, []);
        console.log(JSON.stringify({ result: "ok", browser: browser.version(), viewports: [375, 768, 1920], checks: ["три шрифта", "помощник текста", "сообщение монитора", "сервер доступен"] }));
    }
    finally {
        await browser.close();
    }
})().catch(e => { console.error(e); process.exitCode = 1; });
