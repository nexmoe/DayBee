import { expect, test } from "./fixtures";

test.use({ timezoneId: "Asia/Shanghai" });

for (const role of ["main", "compact"] as const) {
  test(`fills seven elapsed dates across week boundaries in ${role}`, async ({ launchApp }) => {
    const { app, page } = await launchApp();
    for (const [today, dates] of [
      ["2026-10-05", ["2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05"]],
      ["2026-10-08", ["2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08"]],
      ["2026-10-11", ["2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11"]]
    ] as const) {
      await page.clock.setFixedTime(new Date(`${today}T12:00:00+08:00`));
      await page.goto(role === "main" ? "/?window=main" : "/");
      await page.setViewportSize({ width: role === "main" ? 1000 : 420, height: role === "main" ? 760 : 620 });
      const chart = page.getByTestId("speech-activity");
      await expect(chart.getByRole("radio")).toHaveCount(dates.length);
      expect(await chart.getByRole("radio").evaluateAll(elements => elements.map(element => element.getAttribute("data-speech-date")))).toEqual(dates);
      await expect(chart.getByRole("button", { name: /Next week|下一周/, exact: true })).toBeDisabled();
      await expect(chart.getByRole("radio").last()).toHaveAttribute("aria-current", "date");
      await chart.getByRole("radio").nth(1).click();
      expect(await chart.getByRole("radio").evaluateAll(elements => elements.map(element => element.getAttribute("data-speech-date")))).toEqual(dates);
      await expect(chart.getByRole("radio").nth(1)).toBeChecked();
      await chart.getByRole("button", { name: /Previous week|上一周/, exact: true }).click();
      await expect(chart.getByRole("radio")).toHaveCount(7);
      await expect(chart.getByRole("radio").last()).not.toHaveAttribute("data-speech-date", today);
      await chart.getByRole("button", { name: /Next week|下一周/, exact: true }).click();
      await expect(chart.getByRole("radio")).toHaveCount(dates.length);
      expect(await chart.getByRole("radio").evaluateAll(elements => elements.map(element => element.getAttribute("data-speech-date")))).toEqual(dates);
      expect(await chart.locator(".speech-date-strip").evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
    }
    await app.close();
  });
}
