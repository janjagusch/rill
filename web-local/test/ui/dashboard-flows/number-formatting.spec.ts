import {
  createDashboardFromModel,
  waitForDashboard,
} from "../utils/dashboardHelpers";
import { createAdBidsModel } from "../utils/dataSpecifcHelpers";
import { test, expect } from "@playwright/test";
import { startRuntimeForEachTest } from "../utils/startRuntimeForEachTest";
import { updateCodeEditor } from "../utils/commonHelpers";

test.describe("smoke tests for number formatting", () => {
  startRuntimeForEachTest();

  test("smoke tests for number formatting", async ({ page }) => {
    test.setTimeout(60000);
    await page.goto("/");
    // disable animations
    await page.addStyleTag({
      content: `
        *, *::before, *::after {
          animation-duration: 0s !important;
          transition-duration: 0s !important;
        }
      `,
    });
    await createAdBidsModel(page);
    await createDashboardFromModel(page, "AdBids_model");

    // open metrics editor
    await page.getByRole("button", { name: "Edit Metrics" }).click();

    // This is a metrics spec with all available formatting options
    const formatterFlowDashboard = `# Visit https://title: "AdBids_model_dashboard"
model: "AdBids_model"
default_time_range: ""
smallest_time_grain: ""
timeseries: "timestamp"
measures:
- label: humanized default
  expression: count(*)
  name: total_rows
  description: Total number of records present
- label: USD
  expression: sum(bid_price)
  name: total_bid_price
  format_preset: currency_usd
  valid_percent_of_total: true
- label: humanized chosen
  expression: sum(bid_price)
  name: total_humanize
  format_preset: humanize
  valid_percent_of_total: true
- label: No Format
  expression: sum(bid_price)
  name: total_none
  format_preset: none
  valid_percent_of_total: true
- label: percentage
  expression: sum(bid_price)
  name: total_percentage
  format_preset: percentage
  valid_percent_of_total: true
- label: interval_ms
  expression: sum(bid_price)
  name: total_interval_ms
  format_preset: interval_ms
  valid_percent_of_total: true
- label: d3 fixed
  expression: sum(bid_price)
  name: total_d3_fixed_points
  format_d3: ".3f"
  valid_percent_of_total: true
dimensions:
- name: publisher
  label: Publisher
  column: publisher
  description: ""
- name: domain
  label: Domain
  column: domain
  description: ""
`;

    // update the code editor with the new spec
    await updateCodeEditor(page, formatterFlowDashboard);
    // wait for the dashboard to update
    await waitForDashboard(page);

    // make the viewport big enough to see the whole dashboard
    page.setViewportSize({ width: 1920, height: 1200 });

    // Go to dashboard
    await page.getByRole("button", { name: "Go to dashboard" }).click();

    // wait a tick for the dash to update
    await page.waitForTimeout(50);

    /******************
     * check big nums
     ******************/
    for (const [name, bignum, tooltip_num] of [
      ["humanized default", "100.0k", "100000"],
      ["USD", "$300.6k", "300576.83999999857"],
      ["humanized chosen", "300.6k", "300576.83999999857"],
      ["No Format", "300576.83999999857", "300576.83999999857"],
      ["percentage", "30.1M%", "300576.83999999857"],
      ["interval_ms", "5 m", "5m 576ms"],
      ["d3 fixed", "300576.840", "300576.840"],
    ]) {
      // check bignum with correct format exists/is visible
      await expect(
        page.getByRole("button", { name: `${name} ${bignum}` })
      ).toBeVisible();
      // hover over btn_name
      await page.getByRole("button", { name: `${name} ${bignum}` }).hover();
      // wait for a moment for the tooltip to appear
      await page.waitForTimeout(50);
      // check tooltip has correct format
      await expect(page.getByText(`${name} ${tooltip_num}`)).toBeVisible();
    }

    /******************
     * check leaderboard
     *
     * note that the leaderboard is shown with
     * "humanized default" format initially.
     *
     * This is a smoke test, so we won't check every format,
     * but a few combinations of format and context column.
     *
     ******************/
    await page.getByRole("button", { name: "Select a context column" }).click();
    await page
      .getByRole("button", { name: "humanized default", exact: true })
      .click();
    await page.getByRole("menuitem", { name: "USD" }).click();
    // turn on a context column to check the formatting there
    await page.getByRole("button", { name: "Select a context column" }).click();
    await page.getByRole("menuitem", { name: "Percent of total" }).click();

    await expect(
      page.getByRole("button", { name: "null $98.8k 32%" })
    ).toBeVisible();

    await page.getByRole("button", { name: "USD", exact: true }).click();
    await page.getByRole("menuitem", { name: "percentage" }).click();

    await expect(
      page.getByRole("button", { name: "null 9.9M% 32%" })
    ).toBeVisible();

    // try interval_ms...
    await page.getByRole("button", { name: "percentage", exact: true }).click();
    await page.getByRole("menuitem", { name: "interval_ms" }).click();
    // ...and add a time comparison to check absolute change
    await page.getByRole("button", { name: "Select time range" }).click();
    await page.getByRole("menuitem", { name: "Last 4 Weeks" }).click();

    await expect(
      page.getByRole("button", { name: "null 27 s 33%" })
    ).toBeVisible();

    // try No Format...
    await page
      .getByRole("button", { name: "interval_ms", exact: true })
      .click();
    await page.getByRole("menuitem", { name: "No Format" }).click();
    // ...with percent change context column
    await page.getByRole("button", { name: "Select a context column" }).click();
    await page.getByRole("menuitem", { name: "Percent change" }).click();

    // await page.pause();
    await expect(
      page.getByRole("button", { name: "null 26642.549999999974 -14%" })
    ).toBeVisible();

    /******************
     * check dimension table
     *
     * just smoke testing, so we'll just check one value
     * per column.
     ******************/
    await page.locator("#svelte").getByText("Publisher").click();

    // humanized default
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^8\.9k$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // USD
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^\$26\.6k$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // humanized chosen
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^26\.6k$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // No Format
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^26642\.549999999974$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // No Format - context column, delta
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^-4348\.7299999999705$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // No Format - context column, delta pct
    await expect(
      page
        .getByRole("table", { name: "Dimension table" })
        .locator("div")
        .filter({ hasText: /^-14%$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // No Format - context column, pct of total
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^33%$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // percentage
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^383\.4k%$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // interval_ms
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^3\.8 s$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();

    // d3 fixed
    await expect(
      page
        .locator("div")
        .filter({ hasText: /^26642\.550$/ })
        .getByRole("button", { name: "Filter dimension value" })
    ).toBeVisible();
  });
});
