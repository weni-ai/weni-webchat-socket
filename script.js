let j = document.createElement("div");
j.id = "wwc";
document.body.appendChild(j);

let s = document.createElement("link");
s.rel = "stylesheet";
s.href = "";
document.head.appendChild(s);

let p = {
  "title": "rafas local webchat",
  "inputTextFieldHint": "Type a message...",
  "showFullScreenButton": false,
  "displayUnreadCount": false,
  "mainColor": "#009E96",
  "startFullScreen": false,
  "embedded": false,
  "selector": "#wwc",
  "customizeWidget": {
    "headerBackgroundColor": "#009E96",
    "launcherColor": "#009E96",
    "userMessageBubbleColor": "#009E96",
    "quickRepliesFontColor": "#009E96",
    "quickRepliesBackgroundColor": "#009E9633",
    "quickRepliesBorderColor": "#009E96"
  },
  "params": {
    "images": {
      "dims": {
        "width": 300,
        "height": 200
      }
    },
    "storage": "session"
  },
  // "socketUrl": "https://9f08-2804-14d-128a-832f-45ea-8817-b603-4d1a.ngrok-free.app",
  "socketUrl": "http://localhost:8080",
  "host": "https://flows.stg.cloud.weni.ai",
  "channelUuid": "c4dc40fa-37e0-4147-a379-a3e8ffd23f80"
};

p["customMessageDelay"] = message => {
    return 1 * 1000;
}

WebChat.default.init(p);
