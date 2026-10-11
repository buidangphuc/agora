test("x", async ({ page }) => {
  await page.route("**/api/orders", (route) => route.fulfill({ status: 200, body: "{}" }));
  await page.route("http://localhost:8080/v1/**", (route) => route.abort());
});
