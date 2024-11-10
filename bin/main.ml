let verify_signature ~public_key req k =
  let signature = Yume.Server.header (`Raw "x-signature-ed25519") req in
  let signature = Hex.to_string (`Hex signature) in
  let timestamp = Yume.Server.header (`Raw "x-signature-timestamp") req in
  let body = Yume.Server.body req in
  if
    Mirage_crypto_ec.Ed25519.verify ~key:public_key signature
      ~msg:(timestamp ^ body)
  then k ()
  else Yume.Server.respond ~status:`Unauthorized ""

let respond_json json =
  json |> Yojson.Safe.to_string
  |> Yume.Server.respond ~headers:[ (`Content_type, "application/json") ]

(* cf. https://discord.com/developers/docs/interactions/overview#preparing-for-interactions *)
let webhook_handler ~public_key _env req =
  try
    let body = Yume.Server.body req in
    Logs.info (fun m -> m "%s" body);
    match
      body |> Yojson.Safe.from_string |> Yojson.Safe.Util.to_assoc
      |> List.assoc "type"
    with
    | `Int 1 (* PING *) ->
        verify_signature ~public_key req @@ fun () ->
        `Assoc [ ("type", `Int 1 (* PONG *)) ] |> respond_json
    | `Int 2 (* APPLICATION_COMMAND *) ->
        verify_signature ~public_key req @@ fun () ->
        `Assoc
          [
            ("type", `Int 4 (* CHANNEL_MESSAGE_WITH_SOURCE *));
            ( "data",
              `Assoc [ ("content", `String "更新を行います。1 分後にサーバに接続してみてください。") ] );
          ]
        |> respond_json
    | _ ->
        Logs.err (fun m -> m "unknown body: %s" body);
        Yume.Server.respond ~status:`No_content ""
  with e ->
    Logs.err (fun m ->
        m "failed: %s\n%s" (Printexc.to_string e) (Printexc.get_backtrace ()));
    Yume.Server.respond ~status:`Internal_server_error ""

let serve () =
  let discord_application_public_key =
    match Sys.getenv_opt "DISCORD_APPLICATION_PUBLIC_KEY" with
    | Some s -> s
    | None -> failwith "set DISCORD_APPLICATION_PUBLIC_KEY"
  in
  let public_key =
    match
      `Hex discord_application_public_key |> Hex.to_string
      |> Mirage_crypto_ec.Ed25519.pub_of_octets
    with
    | Ok k -> k
    | Error e ->
        Logs.err (fun m ->
            m "failed to load DISCORD_APPLICATION_PUBLIC_KEY: %a"
              Mirage_crypto_ec.pp_error e);
        failwith "failed to load DISCORD_APPLICATION_PUBLIC_KEY"
  in

  Eio_main.run @@ fun env ->
  Eio.Switch.run @@ fun sw ->
  let listen =
    Eio.Net.getaddrinfo_stream ~service:"38000" env#net "localhost" |> List.hd
  in
  Yume.Server.(
    start_server env ~sw ~listen
      (Router.(use [ post "/webhook" (webhook_handler ~public_key) ])
         default_handler))
  @@ fun _socket -> ()

let register_commands guild_id () =
  let application_id =
    match Sys.getenv_opt "DISCORD_APPLICATION_ID" with
    | Some s -> s
    | None -> failwith "set DISCORD_APPLICATION_ID"
  in
  let token =
    match Sys.getenv_opt "DISCORD_TOKEN" with
    | Some s -> s
    | None -> failwith "set DISCORD_TOKEN"
  in

  Eio_main.run @@ fun env ->
  Mirage_crypto_rng_eio.run (module Mirage_crypto_rng.Fortuna) env @@ fun () ->
  Eio.Switch.run @@ fun sw ->
  let endpoint =
    Printf.sprintf
      "https://discord.com/api/v10/applications/%s/guilds/%s/commands"
      application_id guild_id
  in
  let headers =
    [
      ("user-agent", "vahkane");
      ("content-type", "application/json");
      ("authorization", "Bot " ^ token);
    ]
  in
  let body =
    `Assoc
      [
        ("name", `String "factorio");
        ("type", `Int 1 (* SUB_COMMAND *));
        ("description", `String "Operate the factorio server");
        ( "options",
          `List
            [
              `Assoc
                [
                  ("name", `String "operation");
                  ("description", `String "The type of operation");
                  ("type", `Int 3);
                  ("required", `Bool true);
                  ( "choices",
                    `List
                      [
                        `Assoc
                          [
                            ("name", `String "Try update");
                            ("value", `String "try_update");
                          ];
                        `Assoc
                          [
                            ("name", `String "Get current status");
                            ("value", `String "get_status");
                          ];
                      ] );
                ];
            ] );
      ]
    |> Yojson.Safe.to_string
  in
  let resp = Yume.Client.post env ~headers ~body:(`Fixed body) ~sw endpoint in
  Logs.info (fun m ->
      m "response status: %s"
        (Yume.Client.Response.status resp |> Http.Status.to_string));
  Logs.info (fun m -> m "response body: %s" (Yume.Client.Response.drain resp));
  ()

let () =
  Fmt.set_style_renderer Fmt.stderr `Ansi_tty;
  Logs.set_reporter (Logs_fmt.reporter ());
  Logs.set_level (Some Logs.Info);
  Cmdliner.(
    Cmd.(
      group
        ~default:Term.(const serve $ const ())
        (info "vahkane")
        [
          v (info "serve") Term.(const serve $ const ());
          v (info "register-commands")
            Term.(
              const register_commands
              $ Arg.(
                  required & pos 0 (some string) None & info ~docv:"GUILD_ID" [])
              $ const ());
        ]
      |> eval))
  |> exit
