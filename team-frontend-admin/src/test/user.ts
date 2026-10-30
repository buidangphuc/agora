import { act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

/**
 * user-event resolves its own copy of @testing-library/dom, so React Testing
 * Library's act() wrapper is not applied to the events it dispatches and state
 * updates log "not wrapped in act(...)". This helper wraps the calls we use.
 */
export function setupUser() {
  const user = userEvent.setup();
  const wrap =
    <A extends unknown[]>(fn: (...args: A) => Promise<void>) =>
    async (...args: A): Promise<void> => {
      await act(async () => {
        await fn(...args);
      });
    };
  return {
    click: wrap(user.click),
    dblClick: wrap(user.dblClick),
    hover: wrap(user.hover),
    unhover: wrap(user.unhover),
    keyboard: wrap(user.keyboard),
    tab: wrap(user.tab),
    type: wrap(user.type),
    clear: wrap(user.clear),
    selectOptions: wrap(user.selectOptions),
  };
}
