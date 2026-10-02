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
      ] },
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
  missions: 'goals', playground: 'personas', activity: 'how-it-works', approvals: 'safety', settings: 'behaviour',
};

/** "?" help tips next to complex controls. section: where "Learn more" leads. */
export const TIPS = {
  mode: { title: 'Reply mode', section: 'safety',
    text: '**Auto** sends replies by itself. **Approve** holds every reply until you OK it. **Co-pilot** suggests three replies and you pick one.' },
  'dial-speed': { title: 'Speed', section: 'behaviour',
    text: 'How fast it notices a message, thinks and types. **Slow texter** can take many minutes; **Instant** answers within seconds.' },
  'dial-chattiness': { title: 'Chattiness', section: 'behaviour',
    text: 'How often it answers and joins in. Low levels sometimes leave a message on read and cap replies; the top level answers everything, even in busy groups.' },
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
