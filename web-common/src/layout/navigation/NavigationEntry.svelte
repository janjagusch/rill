<script lang="ts">
  import ContextButton from "@rilldata/web-common/components/column-profile/ContextButton.svelte";
  import ExpanderButton from "@rilldata/web-common/components/column-profile/ExpanderButton.svelte";
  import { WithTogglableFloatingElement } from "@rilldata/web-common/components/floating-element";
  import CaretDownIcon from "@rilldata/web-common/components/icons/CaretDownIcon.svelte";
  import MoreHorizontal from "@rilldata/web-common/components/icons/MoreHorizontal.svelte";
  import { Menu } from "@rilldata/web-common/components/menu";
  import { notifications } from "@rilldata/web-common/components/notifications";
  import Tooltip from "@rilldata/web-common/components/tooltip/Tooltip.svelte";
  import TooltipContent from "@rilldata/web-common/components/tooltip/TooltipContent.svelte";
  import { createCommandClickAction } from "@rilldata/web-local/lib/util/command-click-action";
  import { createShiftClickAction } from "../../lib/actions/shift-click-action";
  import { emitNavigationTelemetry } from "./navigation-utils";
  import { currentHref } from "./stores";

  export let name: string;
  export let href: string;
  export let open = false;
  export let expandable = true;
  export let tooltipMaxWidth: string | undefined = undefined;
  export let maxMenuWidth: string | undefined = undefined;
  export let showContextMenu = true;
  // We set `immediatelyNavigate` to false for Sources because we want to confirm with the user before navigating away from an Unsaved Source.
  export let immediatelyNavigate = true;

  const { commandClickAction } = createCommandClickAction();
  const { shiftClickAction } = createShiftClickAction();

  let showDetails = false;
  let contextMenuOpen = false;

  function onShowDetails() {
    showDetails = !showDetails;
  }

  const shiftClickHandler = async () => {
    await navigator.clipboard.writeText(name);
    notifications.send({ message: `copied "${name}" to clipboard` });
  };

  let containerFocused;
  let contextMenuHovered = false;
  let seeMoreHovered = false;

  function onContainerFocus(tf) {
    return () => {
      containerFocused = tf;
    };
  }

  /** track the mousedown event to provide immediate feedback on the click motion. */
  let mousedown = false;
  /** We'll look at the first two segments of the pathname in order to determine
   * if this entry is active. So it should only need to match e.g. /dashboard/<name>,
   * which maintains any subroutes beyond that (like /dashboard/<name>/edit).
   */
  $: pathname = $currentHref.split("/").slice(0, 3).join("/");
  $: isActive = pathname === href;
</script>

<Tooltip
  location="right"
  alignment="start"
  distance={16}
  suppress={contextMenuHovered || contextMenuOpen || seeMoreHovered}
>
  <!-- @ts-ignore -->
  <a
    {href}
    on:click={() => {
      if (immediatelyNavigate) {
        emitNavigationTelemetry(href);
      }
      if (open) onShowDetails();
    }}
    on:mousedown={() => {
      if (immediatelyNavigate) {
        currentHref.set(href);
      }
      mousedown = true;
    }}
    on:mouseup={() => {
      mousedown = false;
    }}
    on:mouseenter={onContainerFocus(true)}
    on:focus={onContainerFocus(true)}
    on:mouseleave={onContainerFocus(false)}
    on:blur={onContainerFocus(false)}
    on:dragend={async () => {
      if (immediatelyNavigate) {
        currentHref.set(href);
      }
      mousedown = false;
    }}
    style:height="24px"
    class:font-bold={isActive}
    class:bg-gray-200={isActive}
    class:bg-gray-100={isActive && mousedown}
    class="
    focus:bg-gray-200
    focus:outline-none
    flex gap-x-[7px] items-center {expandable
      ? 'pl-[11px]'
      : 'pl-8'} pr-2 py-1 {!isActive && !mousedown ? 'hover:bg-gray-100' : ''}"
    use:commandClickAction
    use:shiftClickAction
    on:command-click
    on:shift-click={shiftClickHandler}
  >
    <!-- slot for navigation click -->

    {#if expandable}
      <ExpanderButton
        bind:isHovered={seeMoreHovered}
        rotated={showDetails}
        on:click={onShowDetails}
      >
        <CaretDownIcon size="14px" />
      </ExpanderButton>
    {/if}

    <div
      class="ui-copy flex items-center gap-x-1 w-full text-ellipsis overflow-hidden whitespace-nowrap"
    >
      {#if $$slots["icon"]}
        <div class="text-gray-400" style:width="1em" style:height="1em">
          <slot name="icon" />
        </div>
      {/if}
      <div class:truncate={!$$slots["name"]} class="w-full">
        {#if $$slots["name"]}
          <slot name="name" />
        {:else}
          {name}
        {/if}
      </div>
    </div>
    {#if showContextMenu}
      <!-- context menu -->
      <WithTogglableFloatingElement
        location="right"
        alignment="start"
        distance={16}
        let:toggleFloatingElement
        bind:active={contextMenuOpen}
      >
        <span
          class="self-center"
          class:opacity-0={!containerFocused &&
            !contextMenuOpen &&
            !isActive &&
            !contextMenuHovered}
        >
          <ContextButton
            id="more-actions-{name}"
            tooltipText="More actions"
            suppressTooltip={contextMenuOpen}
            on:click={(event) => {
              /** prevent the link click from registering */
              event.stopPropagation();
              toggleFloatingElement();
            }}
            bind:isHovered={contextMenuHovered}
            width={24}
            height={24}
            border={false}
          >
            <MoreHorizontal />
          </ContextButton>
        </span>
        <Menu
          dark
          maxWidth={maxMenuWidth}
          on:click-outside={toggleFloatingElement}
          on:escape={toggleFloatingElement}
          on:item-select={toggleFloatingElement}
          slot="floating-element"
          let:toggleFloatingElement
        >
          <slot name="menu-items" toggleMenu={toggleFloatingElement} />
        </Menu>
      </WithTogglableFloatingElement>
    {/if}
  </a>
  <!-- if tooltip content is present in a slot, render the tooltip -->
  <div slot="tooltip-content" class:hidden={!$$slots["tooltip-content"]}>
    {#if $$slots["tooltip-content"]}
      <TooltipContent maxWidth={tooltipMaxWidth}
        ><slot name="tooltip-content" /></TooltipContent
      >
    {/if}
  </div>
</Tooltip>

{#if showDetails}
  <slot name="more" />
{/if}
