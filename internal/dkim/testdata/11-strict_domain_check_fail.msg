DKIM-Signature: v=1; a=ed25519-sha256; c=relaxed/relaxed;
    d=example.com; s=selector; t=1709466347;
    i=test@sub.example.com;
    h=from:subject:date:to:message-id:from:subject:date:to:cc:message-id;
    bh=2jUSOH9NhtVGCQWNr9BrIAPreKQjO6Sn7XIkfJVOzv8=;
    b=NDV3SShyaF7fXYoOx9GnBQjFIfsr5bTJUtAwRTk2sTq+5wl/r0uTN1zaSfUWuxYnMIMoSq
      b/xGMFTFmpSbNeCg==;
From: Joe SixPack <joe@football.example.com>
To: Suzie Q <suzie@shopping.example.net>
Subject: Is dinner ready?
Date: Fri, 11 Jul 2003 21:00:37 -0700 (PDT)
Message-ID: <20030712040037.46341.5F8J@football.example.com>

Hi.

We lost the game.  Are you hungry yet?

Joe.

