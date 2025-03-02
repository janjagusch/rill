/** Application-level constants and shared settings
 * ------------------------------------------------
 * spacing, tween lengths, etc.
 */

import { cubicOut as easing } from "svelte/easing";

export const DEFAULT_INSPECTOR_WIDTH = 360;
export const DEFAULT_NAV_WIDTH = 240;
export const DEFAULT_PREVIEW_TABLE_HEIGHT = 400;

/** parameters used in the column profile view & elsewhere */
export const COLUMN_PROFILE_CONFIG = {
  /** The null percentage should be _just_ big enough to show x 100.0%
   * For MD IO 0.4, this is 74px.
   */
  nullPercentageWidth: 44,
  mediumCutoff: 300,
  compactBreakpoint: 300,
  hideRight: 0,
  hideNullPercentage: 0,
  summaryVizWidth: { medium: 68, small: 64 },
  exampleWidth: { medium: 204, small: 132 },
  fontSize: 12,
};

export const TOOLTIP_STRING_LIMIT = 60;

export function collapseInspectorCTAButton(width) {
  return !(width < 398);
}

/** layout constants  */
export const SIDE_PAD = 0;
export const SURFACE_SLIDE_DURATION = 400;
export const LIST_SLIDE_DURATION = 200;
export const SURFACE_SLIDE_EASING = easing;
export const SURFACE_DRAG_DURATION = 50;
