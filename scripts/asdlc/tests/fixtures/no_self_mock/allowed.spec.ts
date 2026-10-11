test("x", async ({ page }) => {
  // asdlc: allow-mock external payment gateway callback
  await page.route("**/api/pay/callback", (route) => route.fulfill({ status: 200 }));
  await page.route("**/cdn/**", (route) => route.fulfill({ status: 200 }));
});
