<!-- @component
A simple menu of actions. When one is clicked, the callback fires,
and the menu closes.

-->
<script lang="ts">
  import { createEventDispatcher, setContext } from "svelte";
  import { WithTogglableFloatingElement } from "../../floating-element";
  import type { Alignment, Location } from "../../floating-element/types";
  import Check from "../../icons/Check.svelte";
  import Spacer from "../../icons/Spacer.svelte";
  import { Divider, Menu, MenuItem } from "../index";
  import type { SelectMenuItem } from "../types";

  export let options;
  export let selection: (SelectMenuItem & { index?: number }) | undefined =
    undefined;

  export let dark: boolean | undefined = undefined;
  export let multiSelect: boolean | undefined = undefined;
  export let location: Location = "bottom";
  export let alignment: Alignment = "start";
  export let distance = 16;
  export let paddingTop: number | undefined = undefined;
  export let paddingBottom: number | undefined = undefined;
  export let minWidth: string | undefined = undefined;
  export let overflowFlipY = false;

  export let active = false;

  if (dark) {
    setContext("rill:menu:dark", dark);
  }

  const dispatch = createEventDispatcher();

  let temporarilySelectedKey;
  function createOnClickHandler(
    main: string,
    right: string,
    description: string,
    key: string,
    disabled = false,
    index: number,
    closeEventHandler: () => void
  ) {
    return async () => {
      if (!multiSelect && isSelected(selection, key)) {
        return;
      }
      selection = { main, right, description, key, disabled, index };
      dispatch("select", selection);

      if (!multiSelect) closeEventHandler();

      temporarilySelectedKey = undefined;
    };
  }

  function isSelected(selection, key) {
    if (multiSelect) {
      return selection?.length && selection?.includes(key);
    }
    return selection === key || selection.key === key;
  }

  /** this function will make the circle check appear briefly before the menu closes */
  $: showCheckJustAfterClick = (key: string) =>
    temporarilySelectedKey !== undefined && temporarilySelectedKey === key;
  /** this function will otherwise render the check if selected, but only
   * if this is not part of the animation ticks
   */
  $: isAlreadySelectedButNotBeingAnimated = (
    key: string,
    isSelected: boolean
  ) => {
    if (multiSelect) return isSelected;
    else return temporarilySelectedKey === undefined && isSelected;
  };
</script>

<WithTogglableFloatingElement
  bind:active
  {location}
  {alignment}
  {distance}
  {overflowFlipY}
  let:handleClose
  let:toggleFloatingElement
>
  <slot {active} {handleClose} toggleMenu={toggleFloatingElement} />

  <Menu
    {minWidth}
    {paddingTop}
    {paddingBottom}
    slot="floating-element"
    let:handleClose
    {dark}
    on:click-outside={() => {
      if (active) handleClose();
    }}
    on:escape={handleClose}
  >
    {#each options as { key, main, description, right, divider = false, disabled = false }, i}
      {@const selected = isSelected(selection, key)}
      <MenuItem
        icon
        animateSelect
        {disabled}
        on:before-select={() => {
          temporarilySelectedKey = key;
        }}
        on:select={createOnClickHandler(
          main,
          right,
          description,
          key,
          disabled,
          i,
          handleClose
        )}
        {selected}
      >
        <svelte:fragment slot="icon">
          {#if showCheckJustAfterClick(key) || isAlreadySelectedButNotBeingAnimated(key, selected)}
            <Check />
          {:else}
            <Spacer />
          {/if}
        </svelte:fragment>
        <div
          class:text-gray-400={disabled}
          class:font-bold={showCheckJustAfterClick(key) ||
            isAlreadySelectedButNotBeingAnimated(key, selected)}
        >
          {main}
        </div>
        <svelte:fragment slot="description">
          {#if description}
            {description}
          {/if}
        </svelte:fragment>
        <svelte:fragment slot="right">
          {right || ""}
        </svelte:fragment>
      </MenuItem>
      {#if divider}
        <Divider marginTop={1} marginBottom={1} />
      {/if}
    {/each}
  </Menu>
</WithTogglableFloatingElement>
