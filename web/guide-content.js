// guide-content.js — every word of the in-app Guide, the "?" help tips and the
// first-run tour, in one place. Plain English for people who have never
// thought about bots. **bold** is the only markup (rendered safely by guide.js
// and help-tip.js). No emoji: pictures come from guide-art.js.

/** Guide sections, in reading order. art: guide-art.js id. */
export const GUIDE_SECTIONS = [
  {
    id: 'welcome', title: 'What Doppel is', short: 'Start here', icon: 'sparkles', art: 'welcome',
    lead: 'Doppel lets a character you create, a persona, answer your WhatsApp chats in its own voice. You choose who it talks to and how it behaves, and you can step in at any moment.',
    blocks: [
      { p: 'Think of it as a puppet show where you write the puppet. Leo the surfer, a dramatic opera singer, a calmer version of you: each persona has a name, a backstory and its own way of texting.' },
      { points: [
        '**It runs on your computer.** Nothing gets installed on your phone.',
        '**It only answers in chats you pick.** Every other chat is left alone.',
        '**You stay in charge.** Read along, approve replies first, or take over whenever you like.',
      ] },
    ],
  },
  {
    id: 'how-it-works', title: 'How it works', short: 'Linked device + AI', icon: 'link', art: 'how',
    lead: 'Doppel joins WhatsApp the same way WhatsApp on a computer does: as a linked device. When a message arrives in a chat you assigned, the persona\'s AI brain writes a reply and Doppel sends it the way a person would.',
    blocks: [
      { steps: [
        '**Link WhatsApp.** Scan a QR code with your phone (WhatsApp › Settings › Linked devices). You can unlink any time from your phone.',
        '**Pick a brain.** Ollama runs a free AI model right on this computer. Or use Claude or OpenAI with your own key.',
        '**Choose a persona.** Use a ready-made one, or describe a character in a sentence and let the AI builder draft it.',
        '**Assign it to a chat.** From then on it notices, reads, thinks, types and replies there, at a human pace.',
      ] },
      { note: 'Replies only go out while your computer is awake and Doppel is running. Closing the browser tab is fine; Doppel keeps working in the background.' },
    ],
  },
  {
    id: 'personas', title: 'Personas', short: 'Characters with a voice', icon: 'personas', art: 'personas',
    lead: 'A persona is a character card: name, picture, bio, personality and texting style. The better the card, the more convincing the replies.',
    blocks: [
      { points: [
        '**Start from a ready-made persona** or press **Build with AI** and describe someone in a sentence. You can change everything afterwards.',
        '**Texting style matters most:** short or long messages, emoji or none, slang, punctuation, which languages.',
        '**Rules** are hard limits, like "never agree to meet" or "never talk about work".',
        '**A daily routine** (optional) gives it a life: at the gym until 8, slow to answer at work, asleep at night.',
        '**Try it in the Playground first.** Chat with any persona privately before it meets a real person.',
      ] },
    ],
  },
  {
    id: 'chats', title: 'Chats and who it answers', short: 'Assigning chats', icon: 'chats', art: 'assign',
    lead: 'A persona only ever speaks in chats you assign. One persona can look after several chats, and each chat can be tuned on its own.',
    blocks: [
      { points: [
        'Open **Chats**, pick a conversation and give it a persona. A switch turns it on or off.',
        'In a **private chat** it answers the other person.',
        'In a **group** it answers when it is talked to (its name, an @mention, a reply to it) and joins in when the AI thinks it fits. In bigger groups you choose **who it answers** in the chat\'s People tab.',
        '**Trigger words** always get a reply; **mute words** never do.',
        '**Languages:** each chat speaks English and Hebrew unless you change it (Settings › Behaviour, or the chat\'s Behaviour tab). Messages clearly in another language get no reply; replies follow the language of the message, or one you pick for the chat or for a person (People tab).',
        '**Nicknames:** in the People tab, give anyone a name to use instead of their WhatsApp name. It is used in every chat, by you and by the persona.',
        'The chat\'s **Log** tab shows every message with what the persona did about it and why: answered (and how long it took), stayed quiet, reacted or left it for you. On top: how often it answers, its typical reply time and who it answers most.',
        '**Testing tip:** in an assigned chat, send a message starting with **1** (your test trigger, for example "1 hi") and the persona answers you straight away. "You (message yourself)" is the safest place to try.',
      ] },
    ],
  },
  {
    id: 'behaviour', title: 'Vibe dials and behaviour', short: 'How it replies', icon: 'sliders', art: 'dials',
    lead: 'Three dials set the feel of a persona. Turn one and the reply timeline below it shows what changes.',
    blocks: [
      { points: [
        '**Speed:** from Slow texter (checks the phone now and then) to Instant (answers in seconds).',
        '**Chattiness:** from Reserved (sometimes leaves a message on read and caps how often it replies) to Answers everything.',
        '**Boldness:** emoji reactions, quoting messages, tagging people, and the odd typo it then fixes, like a real person.',
      ] },
      { p: '**Presets** set all three at once: Natural, Instant, Busy, Slow texter and Night owl.' },
      { p: 'Want every detail? Turn on **Advanced** to see all of it: active hours, message splitting, reply length, limits, check-ins and trick protection. Rows a dial controls carry its colour; change one by hand and the dial shows **tuned**.' },
      { note: 'Settings › Behaviour holds the defaults for private chats and for groups. Every chat can have its own behaviour on top (chat panel › Behaviour).' },
    ],
  },
  {
    id: 'goals', title: 'Goals, missions and openers', short: 'Something to aim for', icon: 'target', art: 'missions',
    lead: 'A goal is something the persona quietly works towards, like getting a friend to say a word or making real plans to meet.',
    blocks: [
      { points: [
        '**Missions** are ready-made goals with blanks to fill in. Pick one on the Missions page or in a chat\'s Goal tab. Completed missions earn badges.',
        '**Style:** Subtle (secret and gradual), Balanced (may ask about it naturally) or Direct (goes for it).',
        '**Plan ahead** lets the AI think one private step ahead before each reply. Smarter steering, slightly slower replies.',
        'Doppel only counts a goal as reached with proof in their messages, then celebrates it in the activity feed.',
        '**Start a conversation** sends an opener now, so the persona doesn\'t have to wait for them to write first.',
        '**Scheduled messages** (Behaviour › Start conversations › On a schedule): an opener when replies are turned on or you join a group, one at each set time, and repeats: a fixed gap, an even spread, a bell curve or at random (Poisson).',
        'A scheduled message only goes out if it can go right then. If WhatsApp is disconnected, you left the group, the chat is Away or outside active hours, that one is skipped and never sent later, so messages never pile up.',
      ] },
    ],
  },
  {
    id: 'tasks', title: 'Tasks: finding a meeting time', short: 'Errands with a result', icon: 'calendar', art: 'missions',
    lead: 'A task is an errand with a clear result, like agreeing on a meeting time with Dana next week. The persona does the talking; code decides which times to offer and checks every message before it goes.',
    blocks: [
      { steps: [
        '**Start one** on the Tasks page: pick a private chat (any contact; one without a persona gets the one you choose when the task starts), say what the meeting is, how long, which days and hours, and how far ahead.',
        '**It offers times.** Round times only, never right next to something busy, picked by code. The AI only ever sees the times it may say.',
        '**They answer.** Each reply is read into plain facts (they agree, they suggest Tuesday at 10, they ask why not…), then the first rule that fits decides the next message.',
        '**You confirm.** When they say yes, it checks your calendar once more and asks you on the Tasks page (with a notification). Then it confirms in the chat and books it, if you chose a calendar.',
      ] },
      { points: [
        '**Your calendar stays private.** No titles, places or free/busy list ever reach the AI. A time they ask about gets "works" or "doesn\'t work", never a reason, and the same answer every time.',
        '**Strategies:** **They go first** (asks for their times and offers one match: shows the least), **A few times**, or **Parts of days**.',
        '**Probing:** after a few "doesn\'t work" answers it stops answering time by time and sends options, asks for their times, or hands the chat to you with a summary.',
        '**"Why not then?"** It brushes it off, or uses one of your own lines. Settings › Tasks can draft lines from a short description.',
        '**Every draft is checked** for times it may not mention, words from your calendar and reasons for being busy. A draft that fails twice is replaced by a safe message written by code.',
        '**Never money.** Anything about payment is brushed off and handed to you.',
        '**Limits:** a message limit, a number of days and nudges when they go quiet. Then it hands the chat to you, paused, with what happened so far.',
        '**Try it first:** an AI plays the other person (easy-going, vague, busy or nosy). Nothing is sent or booked.',
        '**Secretaries** start a Find a time task by themselves when someone asks for a meeting, with the secretary\'s meeting days and hours.',
        '**Advanced:** edit the rule list: when something happens, what to do next. Conditions and actions come from a fixed list, so every rule can be checked.',
      ] },
      { note: 'Tasks run in private chats for now. While a task runs it is the chat\'s goal: check-ins and scheduled messages wait until it ends.' },
    ],
  },
  {
    id: 'safety', title: 'Approvals, co-pilot, hand-off and reveal', short: 'Staying in control', icon: 'shield', art: 'modes',
    lead: 'You decide how much the persona does on its own. Every chat has a mode:',
    blocks: [
      { points: [
        '**Auto:** replies go out by themselves, with human-like timing.',
        '**Approve:** every reply waits in Approvals until you send it, change it or skip it.',
        '**Co-pilot:** it suggests three replies (brief, warm, playful) and you pick one or write your own.',
      ] },
      { figure: 'handoff', title: 'Hand-off', text: 'If someone brings up money, health, meeting up, distress or legal trouble, or asks whether they are talking to a bot, the persona goes quiet and Doppel asks you to take over from your phone. Press Resume when you are done.' },
      { figure: 'reveal', title: 'Reveal', text: 'Sends a friendly message that it was an AI persona all along, then pauses the chat. Use it when the joke has run its course. You can edit the message in Settings › Safety.' },
      { points: [
        '**Trick protection** blocks messages that try to make the persona drop its act.',
        '**Away** pauses a chat for a while; **active hours** keep it to certain times of day.',
      ] },
    ],
  },
  {
    id: 'memory', title: 'Memory and privacy', short: 'What it keeps', icon: 'brain', art: 'memory',
    lead: 'Personas can remember small things people mention, like an exam on Friday or a dog called Pita, and bring them up later the way a friend would.',
    blocks: [
      { points: [
        'Memories are kept per chat. See, pin, change or delete them in the chat\'s Memory tab, or turn learning off.',
        '**Across chats:** a persona can use what someone told it privately to stay consistent in a group — **discreetly by default**: it never brings private things up unless that person does. Private chats can mention what happened in groups you share. Sensitive topics (money, health, relationships, secrets) never cross over, and you can lock any memory to one chat in the Memory tab.',
        'Everything lives on this computer in Doppel\'s data folder: settings, personas, chats, history and memories.',
        'With **Ollama**, nothing leaves your computer. With **Claude** or **OpenAI**, the chat text needed for a reply is sent to that company.',
        'Clear a chat\'s history or memories from its panel any time. Removing a chat also deletes what it remembered.',
      ] },
    ],
  },
  {
    id: 'context', title: 'Context systems', short: 'How it thinks', icon: 'flow', art: 'memory',
    lead: 'A context system decides when a persona thinks with the AI and what it reads before it writes: the rules for joining in, how often it plans a goal, what checks a reply, and how big each part of its prompt may be.',
    blocks: [
      { points: [
        '**Three are built in.** **Classic** is how replies always worked: an AI check on every group message and a goal plan before every reply (about 4 AI calls before a group reply). **Lean** settles clear cases in code, checks every reply in code, plans every few replies and reads a short persona card (about 1.7 calls). **Fast** writes each reply with one call.',
        '**Choose per chat.** The app has a default, a persona can have its own, and any chat can pick another one: Context › Who uses it, or the chat\'s Memory tab.',
        '**Make your own.** Copy a built-in system, then tune its settings or draw its flowchart: drag blocks, wire them, add your own steps and notes. Or describe a system in words and let the AI draw it. Only the settings change how chats run; "Match settings to chart" reads them off your drawing.',
        '**Try before you switch.** The sandbox answers a chat\'s last messages again with two or three systems, side by side: what each would have said, how many AI calls it took and how long. Nothing is sent.',
        '**Checking replies** catches nonsense before it is sent: copied instructions, a private plan sent as the message, repeats of its last replies, the wrong language. A failing reply is written again with the reason; if it still fails, the persona stays quiet. The Log tab says why.',
      ] },
    ],
  },
  {
    id: 'faq', title: 'Questions', short: 'FAQ', icon: 'info', art: 'faq',
    lead: 'Quick answers to the things people ask most.',
    blocks: [
      { qa: [
        ['Ollama, Claude or OpenAI?', 'Ollama is free and private: the AI runs on your computer (16 GB of memory or more works best, and llama3.1:8b is a good first model). Claude and OpenAI usually write more natural replies; they need an API key, charge per use and see the messages they answer.'],
        ['Does my phone have to stay on?', 'No. Linked devices keep working while your phone is off for a while. Doppel itself needs your computer awake and the app running.'],
        ['Can I change how it looks?', 'Yes. Settings › Appearance has nine looks: Liquid Glass, pure-black Midnight, pure-white Daylight, flat Classic, paper-and-ink Vintage, Ocean, Forest, Neon and High contrast, each with a live preview, plus a light/dark switch. The choice is saved with your settings, so every browser on this computer shows the same look.'],
        ['How do I quit?', 'Settings › About › Quit Doppel. Personas stop replying until you open the app again, and nothing is deleted.'],
        ['The page says the port is in use.', 'Settings › Server › Find free ports, pick one and press the button to switch. The page moves to the new address by itself and remembers it.'],
        ['Where is my data?', 'In one folder in your home folder. Settings › Data shows exactly where and opens it for you (on a Mac it is Library › Application Support › WhatsappDoppel, on Windows AppData › Roaming › WhatsappDoppel, on Linux .config › WhatsappDoppel). Deleting that folder starts completely fresh.'],
        ['Will people notice it is not me?', 'They might, and with friends who are in on it that is half the fun. Doppel takes its time, types like a person and stays in character, but it is still an AI. The reveal message ends things nicely.'],
        ['Is this allowed by WhatsApp?', 'Automated messaging can break WhatsApp\'s terms, and accounts that act like bots can be restricted. Keep volume low, use realistic timing, leave check-ins off, and keep it to friends. The risk is yours.'],
      ] },
    ],
  },
  {
    id: 'responsible', title: 'Using it kindly', short: 'Ground rules', icon: 'heart', art: 'kind',
    lead: 'Doppel speaks as you, from your account. That is a lot of fun with friends who are in on the joke, and a real way to hurt people if they are not.',
    blocks: [
      { points: [
        '**Use it with people who would laugh about it afterwards.** Pranks among friends, a group game, testing a character you are writing: great. Fooling someone vulnerable, a partner, a colleague or a stranger: not okay.',
        '**End it well.** Reveal yourself when the joke has run its course, and stop if someone seems confused, upset or emotionally invested.',
        '**Never use it for** scams, harassment, romance deception, pretending to be someone else, or collecting personal information.',
        '**Stay in control:** use Approve mode for anyone you don\'t know well, and keep hand-off on.',
        'You are responsible for what is sent from your account. Doppel is free for personal, non-commercial use and is not affiliated with WhatsApp or Meta.',
      ] },
    ],
  },
];

/** Which guide section the topbar "?" opens on each page. */
export const PAGE_SECTION = {
  welcome: 'welcome', dashboard: 'welcome', chats: 'chats', personas: 'personas', persona: 'personas',
  missions: 'goals', tasks: 'tasks', context: 'context', playground: 'personas', activity: 'how-it-works', approvals: 'safety', settings: 'behaviour',
};

/** "?" help tips next to complex controls. section: where "Learn more" leads. */
export const TIPS = {
  mode: { title: 'Reply mode', section: 'safety',
    text: '**Auto** sends replies by itself. **Approve** holds every reply until you OK it. **Co-pilot** suggests three replies and you pick one.' },
  'dial-speed': { title: 'Speed', section: 'behaviour',
    text: 'How fast it notices a message, thinks and types. **Slow texter** can take many minutes; **Instant** answers within seconds.' },
  'dial-chattiness': { title: 'Chattiness', section: 'behaviour',
    text: 'How often it answers and joins in. Low levels sometimes leave a message on read and cap replies; the top level answers everything, even in big groups, where it would otherwise answer only the people you pick. Picked people mode and anyone you switched off still win.' },
  'dial-boldness': { title: 'Boldness', section: 'behaviour',
    text: 'How expressive it is: emoji reactions, quoting messages and tagging people in groups, and the odd typo it then corrects.' },
  advanced: { title: 'Advanced', section: 'behaviour',
    text: 'Every setting, grouped: timing, active hours, message shape, limits, check-ins and trick protection. Rows with a coloured dial label are set by that dial.' },
  presets: { title: 'Presets', section: 'behaviour',
    text: 'A preset sets all three dials and the advanced settings in one go. Pick one, then fine-tune with the dials.' },
  reach: { title: 'Reachable, slow or away', section: 'personas',
    text: 'During this part of its day the persona answers as usual (**Reachable**), takes longer to notice (**Slow**), or waits until it is over (**Unreachable**) and may mention where it was.' },
  handoff: { title: 'Hand-off', section: 'safety',
    text: 'When someone brings up something serious (money, health, meeting up, distress, legal trouble) or asks if it is a bot, the persona goes quiet and Doppel asks you to take over.' },
  languages: { title: 'Languages', section: 'chats',
    text: 'The languages the persona speaks here (English and Hebrew unless you change it). Messages clearly in another language get no reply unless you choose **Answer**. Replies use the language of the message, or one you pick, for the chat or per person.' },
  'auto-continue': { title: 'Keep going when flagged', section: 'safety',
    text: 'Normally a message about money, health, meeting up or asking if it is a bot pauses the chat until you tap Resume. With this on, such messages are only flagged and the persona keeps replying.' },
  context: { title: 'What it reads', section: 'memory',
    text: 'Everything the persona gets for its next reply here: its prompt, your instructions for this chat, a summary of older messages and the recent messages. Edit the instructions or the summary, summarise now, or start fresh.' },
  context: { title: 'Context systems', section: 'context',
    text: 'How a persona decides when to use the AI and what it reads: joining in, goal planning, checks on each reply, background work and prompt size. Classic, Lean and Fast are built in; copy one to tune it or draw your own flowchart.' },
  'ctx-sandbox': { title: 'Sandbox', section: 'context',
    text: 'Answers a chat\'s last messages from them again with each system you pick, like the chat would, and counts every AI call. Nothing is sent and the chat is left as it was. Real AI calls, so it takes a while on a local model.' },
  'ctx-card': { title: 'Persona card', section: 'context',
    text: 'A short version of the persona written once by the AI, used instead of the full text by systems with "Short persona card". Smaller prompts are faster and small models follow them better. Edit it freely; changing the persona makes a new one.' },
  compact: { title: 'Long-term memory', section: 'memory',
    text: 'Each chat reads its recent messages word for word (at least 30). Every 70 new messages (200 in a busy chat) the older ones are folded into a short summary it keeps reading, so it remembers far back without a huge prompt. You can read and fix the summary in the chat\'s Memory tab.' },
  ratings: { title: 'Connection', section: 'chats',
    text: 'Once a day, when a group had at least 20 messages, the persona privately scores how connected it felt to each person who wrote (0 to 100) and says why in its own words. Only you see it.' },
  log: { title: 'Log', section: 'chats',
    text: 'Every message in this chat with what the persona did about it: answered (and how long it took), stayed quiet (and why), reacted or left it for you. Tap a row to see each step. Live progress shows a reply being written as it happens: planning, the AI model writing, checks and typing, with how long each took.' },
  memory: { title: 'Memory', section: 'memory',
    text: 'Lets the persona remember small facts people mention and bring them up later. Stored only on this computer; you can see and delete every memory.' },
  cross: { title: 'Context from other chats', section: 'memory',
    text: '**Discreet** knows but never tells. **Open** may refer to it with that person only. **Off** keeps chats separate. Sensitive things never cross.' },
  reveal: { title: 'Reveal', section: 'safety',
    text: 'Sends a friendly message that it was an AI persona all along, then pauses the chat. You can edit the message in Settings › Safety.' },
  clone: { title: 'Clone yourself', section: 'memory',
    text: 'With your OK, Doppel keeps a private sample of messages **you** write, so it can draft a persona that texts like you. Stored only on this computer; delete it any time.' },
  'plan-ahead': { title: 'Plan ahead', section: 'goals',
    text: 'Before each reply the AI privately thinks about the next step towards the goal. Smarter steering, slightly slower replies.' },
  'who-answers': { title: 'Who it answers', section: 'chats',
    text: 'In small groups it answers everyone. In bigger ones it answers the people you pick, plus anyone who says its name or tags it.' },
  tasks: { title: 'Tasks', section: 'tasks',
    text: 'An errand with a result, like agreeing on a meeting time. Code picks the times and checks every message; the AI only reads their answers and writes yours. Nothing is confirmed without you, unless you say so.' },
  'task-strategy': { title: 'How it shows your time', section: 'tasks',
    text: '**They go first** asks when suits them and offers one match. **A few times** offers one to three it picked. **Parts of days** offers "Tuesday morning" first. None of them ever shows your whole week.' },
  'task-cover': { title: '"Why not then?"', section: 'tasks',
    text: 'It never gives the real reason. **Brush it off** says something else is on; **Use my lines** sends one of the lines you wrote, nothing more.' },
  'task-confirm': { title: 'Confirming', section: 'tasks',
    text: '**Ask me first** waits for your OK on the Tasks page. **By itself if free** confirms when the time is still free, and tells you. Either way it checks the calendar once more first.' },
  'task-rules': { title: 'The rules', section: 'tasks',
    text: 'From the top down, the first rule that fits what just happened decides the next message. Pick conditions and actions from the lists; reorder with the arrows.' },
  'task-try': { title: 'Try it first', section: 'tasks',
    text: 'An AI plays the other person for a few messages so you can see how the task would go. Nothing is sent, saved or booked.' },
  filter: { title: 'Content filter', section: 'chats',
    text: 'Code reads every reply before it goes out, and again after it is translated. Curse words are taken out. A reply with a slur, or about suicide or self-harm, is written again once and otherwise not sent.' },
  character: { title: 'Keep it in character', section: 'chats',
    text: 'When a reply makes a claim about the persona itself, code compares it with the persona\'s profile. If it sounds like someone else\'s life, it is written again once and otherwise not sent. This needs the small nomic-embed-text model in Ollama; without it this check is skipped.' },
  'task-calendar': { title: 'Use my calendars', section: 'tasks',
    text: 'Takes the busy times of the calendars in Settings › Calendars out of the times it offers. Entries without an end time (reminders) never count as busy.' },
  missions: { title: 'Missions', section: 'goals',
    text: 'Ready-made goals with blanks to fill in. The persona works on it quietly; when it happens, the mission is complete and badges unlock.' },
  looks: { title: 'Looks', section: 'faq',
    text: 'Pick how Doppel looks: **Liquid Glass**, pure-black **Midnight**, pure-white **Daylight**, flat **Classic**, paper-and-ink **Vintage** and more. Some looks are always dark or always light; the rest follow **Light or dark**.' },
  'goal-style': { title: 'Goal style', section: 'goals',
    text: '**Subtle** keeps it secret and gradual. **Balanced** may ask about it naturally. **Direct** goes for it openly.' },
};

/** First-run tour. target: CSS selector; the step is skipped when it is missing. */
export const TOUR_STEPS = [
  { target: '.nav-item[data-key="chats"]', title: 'Chats',
    text: 'Pick a conversation and give it a persona. It only ever talks where you say so.' },
  { target: '.nav-item[data-key="personas"]', title: 'Personas',
    text: 'Characters with their own voice. Make one in a minute with the AI builder, then try it in the Playground.' },
  { target: '.nav-item[data-key="missions"]', title: 'Missions',
    text: 'Playful goals for a persona, like getting a friend to say a word. Complete them to earn badges.' },
  { target: '.nav-item[data-key="approvals"]', title: 'Approvals',
    text: 'In Approve or Co-pilot mode, replies wait here until you send, change or skip them.' },
  { target: '.nav-item[data-key="settings"]', title: 'Settings',
    text: 'Turn the vibe dials to set how fast, chatty and bold personas are. Advanced holds every detail.' },
  { target: '[data-tour="help"]', title: 'Help is always here',
    text: 'Press ? for the guide on whatever page you are on. You can replay this tour from the Guide.' },
];
