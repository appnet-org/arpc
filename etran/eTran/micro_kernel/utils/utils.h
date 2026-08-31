#pragma once

#include <sys/wait.h>

static inline bool exec_cmd(const std::string &cmd, std::string &result)
{
    std::FILE *pipe = popen(cmd.c_str(), "r");
    if (!pipe)
        return false;

    char buffer[128];
    result.clear();
    while (fgets(buffer, sizeof(buffer), pipe) != nullptr)
    {
        result += buffer;
    }

    int status = pclose(pipe);
    return status >= 0 && WIFEXITED(status) && WEXITSTATUS(status) == 0;
}

static inline bool exec_cmd(const std::string &cmd)
{
    std::string unused_result;
    return exec_cmd(cmd, unused_result);
}