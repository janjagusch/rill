<script lang="ts">
  import Portal from "@rilldata/web-common/components/Portal.svelte";
  import { getContext } from "svelte";
  import type { Writable } from "svelte/store";
  import { DEFAULT_INSPECTOR_WIDTH } from "../config";
  import { drag } from "../drag";
  import type { LayoutElement } from "./types";

  /** the core inspector width element is stored in localStorage. */
  const inspectorLayout = getContext(
    "rill:app:inspector-layout"
  ) as Writable<LayoutElement>;

  const inspectorWidth = getContext(
    "rill:app:inspector-width-tween"
  ) as Writable<number>;

  const visibilityTween = getContext(
    "rill:app:inspector-visibility-tween"
  ) as Writable<number>;
</script>

<div
  class="fixed"
  aria-hidden={!$inspectorLayout.visible}
  style:right="{$inspectorWidth * $visibilityTween}px"
  style:top="var(--header-height)"
>
  <div
    class="
      bg-white
        border-l
        border-gray-200
        fixed
        overflow-auto
        transition-colors
        h-screen
      "
    class:hidden={$visibilityTween === 0}
    class:pointer-events-none={!$inspectorLayout.visible}
    style:width="{$inspectorWidth}px"
  >
    <!-- draw handler -->
    {#if $inspectorLayout.visible}
      <Portal>
        <div
          role="separator"
          class="fixed drawer-handler w-4 hover:cursor-col-resize translate-x-2 h-screen"
          style:right="{$visibilityTween * $inspectorWidth}px"
          style:top="var(--header-height)"
          style:bottom="0px"
          use:drag={{ minSize: 300, store: inspectorLayout, reverse: true }}
          on:dblclick={() => {
            inspectorLayout.update((state) => {
              state.value = DEFAULT_INSPECTOR_WIDTH;
              return state;
            });
          }}
        />
      </Portal>
    {/if}

    <div class="w-full pt-2">
      <slot />
    </div>
  </div>
</div>
