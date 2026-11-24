---
title: "The mailbox"
description: "The email grid - inboxes, live updates, sorting, bulk actions, keyboard shortcuts and the mobile layout."
---

# The mailbox

The mailbox is the main screen: a sidebar with your inboxes and a grid with the captured emails.

![The mailbox](/img/screenshots/mailbox.png)

## Layout

| Area | What you find there |
|---|---|
| **Top bar** | The **Live** indicator, language selector, theme toggle (system / light / dark), settings, your user and sign out. |
| **Sidebar** | **All** plus one entry per inbox you can see, each with its unread count (badge) and total. At the bottom: SMTP port, retention and the server version. |
| **Header** | Current inbox and number of emails, **Refresh**, **Mark all read** and **Empty**. |
| **Search row** | The [search box](search.md) (with a help button), the All / Unread / Read selector and the classic **Filters** panel. |
| **Grid** | Received time, From, To, Subject (with tags and a text preview), Inbox, attachment icon and size. Unread emails are bold with a dot. |
| **Footer** | Range shown, page size (up to 500 per page) and pagination. |

## Live updates

The UI keeps a Server-Sent Events connection open. New emails appear in the grid as soon as they are received,
counters update, and a toast announces them. The dot in the top bar shows the state: **Live**, **Reconnecting…**
or **Offline**. With several PostDream replicas, events travel between them through PostgreSQL, so every browser
sees every email ([Scaling](../operations/scaling.md)).

Turn on **desktop notifications** in [Preferences](preferences.md) to be notified while the tab is in the background.

## Sorting and paging

Click a column header to sort by received date, subject, from, to, size or inbox; click again to flip the order.

## Selecting and bulk actions

Tick the checkboxes (or the header checkbox to select the whole page) and a bulk bar appears:

- **Read / Unread**: mark the selection.
- **Tag**: add or remove tags on all selected emails.
- **Delete**: delete the selection.

![Tagging several emails at once](/img/screenshots/bulk-tag.png)

**Mark all read** marks every email of the current inbox. **Empty** deletes every email of the current inbox, or,
when a search or filter is active, **only the emails that match it**. It always asks for confirmation.

## Keyboard shortcuts

| Key | Action |
|---|---|
| `j` or `↓` / `k` or `↑` | Open the next / previous email |
| `Esc` | Close the open email |
| `Del` | Delete the open email |

## Dark mode and mobile

The UI follows your operating system's light or dark theme by default; you can force one with the theme button.
On narrow screens the sidebar becomes a menu and the grid shows the essential columns.

| Dark mode | Phone |
|---|---|
| ![Dark mode](/img/screenshots/mailbox-dark.png) | ![Mobile layout](/img/screenshots/mobile.png) |
